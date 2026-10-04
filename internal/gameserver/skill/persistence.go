package skill

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/rs/zerolog"
)

type skillSaveStore interface {
	Replace(ctx context.Context, charObjID int32, classIndex int32, rows []effect.SaveRow) error
	ListByCharacter(ctx context.Context, charObjID int32, classIndex int32) ([]effect.SaveRow, error)
	DeleteByCharacter(ctx context.Context, charObjID int32, classIndex int32) (int64, error)
}

type skillLevelStore interface {
	ListKnownSkills(ctx context.Context, charObjID int32, classIndex int32) (player.SkillLevels, error)
}

type skillLevelWriter interface {
	SetKnownSkill(ctx context.Context, charObjID int32, classIndex int32, skillID int, level int) error
}

type skillLevelDeleter interface {
	DeleteKnownSkill(ctx context.Context, charObjID int32, classIndex int32, skillID int) error
}

// Persistence saves and restores a live player's buff and skill-reuse state.
type Persistence struct {
	store  skillSaveStore
	levels skillLevelStore
	skills *modelskill.Table
	now    func() time.Time
	// worker runs character_skills writes on the character's lane, off the
	// actor queue that learned the skill. A nil worker writes inline.
	worker             *persist.Worker
	log                zerolog.Logger
	storeSkillCooltime atomic.Bool
	// augments resolves an augmented weapon's stat bonuses; nil grants
	// none. Set once at boot, before any character is in the world.
	augments *augmentation.Table
	// armorSets grants a worn armor set's skills; nil grants none. Set
	// once at boot, before any character is in the world.
	armorSets *armorset.Table
}

// NewPersistence returns a lifecycle persistence component backed by store and
// the loaded skill table.
func NewPersistence(store skillSaveStore, skills *modelskill.Table, levels ...skillLevelStore) *Persistence {
	return NewPersistenceWithClock(store, skills, time.Now, levels...)
}

// NewPersistenceWithStoreSkillCooltime returns persistence configured for the
// server's StoreSkillCooltime setting.
func NewPersistenceWithStoreSkillCooltime(store skillSaveStore, skills *modelskill.Table, enabled bool, levels ...skillLevelStore) *Persistence {
	p := NewPersistence(store, skills, levels...)
	p.SetStoreSkillCooltime(enabled)
	return p
}

// NewPersistenceWithClock returns a lifecycle persistence component using now
// as its time source.
func NewPersistenceWithClock(store skillSaveStore, skills *modelskill.Table, now func() time.Time, levels ...skillLevelStore) *Persistence {
	p := &Persistence{store: store, skills: skills, now: now, log: zerolog.Nop()}
	p.storeSkillCooltime.Store(true)
	if len(levels) > 0 {
		p.levels = levels[0]
	}
	return p
}

// SetPersistWorker routes character_skills writes onto w, keyed by the
// character, and logs a failed write to log. Without it the writes run on the
// calling goroutine.
func (p *Persistence) SetPersistWorker(w *persist.Worker, log zerolog.Logger) {
	if p != nil {
		p.worker, p.log = w, log
	}
}

// SetAugmentations makes the bonuses of t's augmentations apply while an
// augmented weapon is worn. Call it once at boot, before any character
// enters the world. It fails when a bonus names a stat that does not exist.
func (p *Persistence) SetAugmentations(t *augmentation.Table) error {
	if t != nil {
		for _, name := range t.BonusStats() {
			if _, err := stat.ByName(name); err != nil {
				return fmt.Errorf("augmentation bonus: %w", err)
			}
		}
	}
	p.augments = t
	return nil
}

// SetArmorSets makes a worn armor set grant its skills. Call it once at
// boot, before any character enters the world.
func (p *Persistence) SetArmorSets(t *armorset.Table) {
	if p != nil {
		p.armorSets = t
	}
}

// SetStoreSkillCooltime controls persistence of effects and reuse timers.
func (p *Persistence) SetStoreSkillCooltime(enabled bool) {
	if p != nil {
		p.storeSkillCooltime.Store(enabled)
	}
}

// SaveState is a character's persisted skill state copied at one instant, so
// the write can run later without reading the live character. The zero value
// writes nothing.
type SaveState struct {
	charID     int32
	classIndex int32
	rows       []effect.SaveRow
	ok         bool
}

// SaveState copies c's current active effects and pending reuse timers. A
// character in a duel saves nothing: the rows its duel saved when it began
// stay until the duel restores them (DuelState).
func (p *Persistence) SaveState(c *player.Character) SaveState {
	if c != nil && c.InDuel() {
		return SaveState{}
	}
	return p.snapshot(c)
}

// snapshot is SaveState whatever c's duel.
func (p *Persistence) snapshot(c *player.Character) SaveState {
	if p == nil || !p.storeSkillCooltime.Load() || p.store == nil || c == nil {
		return SaveState{}
	}
	classIndex := c.SkillSaveClassIndex()
	// Reuse expiries and effect periods run on c's queue clock, so the save
	// reads that clock rather than p.now.
	now := c.Now()
	// Effects a login restored but has not replayed yet, when the session
	// ends before entering the world, are saved back with the time since
	// the restore counted: the restore consumed their rows.
	effects := append(p.liveActiveEffects(c, now), p.stagedActiveEffects(c, now)...)
	rows := effect.BuildSaveRows(effects, c.SkillReuseTimers(now), classIndex)
	return SaveState{charID: c.ID, classIndex: classIndex, rows: rows, ok: true}
}

// Save replaces the character's persisted skill state with st.
func (p *Persistence) Save(ctx context.Context, st SaveState) error {
	if !st.ok {
		return nil
	}
	if err := p.store.Replace(ctx, st.charID, st.classIndex, st.rows); err != nil {
		return fmt.Errorf("save skill state for character %d: %w", st.charID, err)
	}
	return nil
}

// liveActiveEffects snapshots c's live effect list into the ActiveEffect view
// BuildSaveRows needs, mirroring Player.storeEffect()'s use of
// getAllEffects(): the effect list itself is the single source of truth for
// what gets saved, not a separate write-only registry. A stacked-out effect
// still held in the list is saved too, so it resumes behind the stronger one
// after relog. An effect whose skill definition no longer resolves is
// dropped, matching a stale datapack change.
func (p *Persistence) liveActiveEffects(c *player.Character, now time.Time) []effect.ActiveEffect {
	list := c.EffectList()
	if list == nil {
		return nil
	}
	var out []effect.ActiveEffect
	for _, e := range list.All() {
		ref := modelskill.Ref{ID: e.Skill.ID, Level: e.Level}
		def, ok := p.definition(ref)
		if !ok {
			continue
		}
		count, elapsed := e.SaveState(now)
		out = append(out, effect.ActiveEffect{
			Skill:        ref,
			ReuseGroup:   cast.ReuseKey(def),
			Count:        count,
			Time:         elapsed,
			Toggle:       e.Skill.Toggle,
			Herb:         e.Herb,
			Continuous:   e.Skill.SkillType == "CONT",
			HealOverTime: e.Type == effect.TypeHealOverTime,
		})
	}
	return out
}

// stagedActiveEffects is the save view at now of the effects a restore
// staged on c and ReplayEffects has not reinstated yet: each has run from
// its restore instant, as it would on the live effect list, and one that
// has ended by now is left out (see effect.RestoredSaveState).
func (p *Persistence) stagedActiveEffects(c *player.Character, now time.Time) []effect.ActiveEffect {
	staged := c.ActiveSkillEffects()
	out := staged[:0]
	for _, eff := range staged {
		if def, ok := p.definition(eff.Skill); ok {
			count, elapsed, alive := effect.RestoredSaveState(def.Effects, eff.Count, eff.Time, eff.RestoredAt, now)
			if !alive {
				continue
			}
			eff.Count, eff.Time = count, elapsed
		}
		out = append(out, eff)
	}
	return out
}

// RestoreKnownSkills restores learned skills independently from effects and reuse timers.
func (p *Persistence) RestoreKnownSkills(ctx context.Context, c *player.Character) error {
	if p == nil || c == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	return p.restoreKnownSkills(ctx, c, classIndex)
}

// RestoreSkillState consumes persisted effects and reuse timers: it stages
// them on c, then deletes their rows. Call it once c is attached and
// nothing else in the login can fail, so a login that does not complete
// leaves the rows in place; from here on the session's end saves the
// staged state back.
func (p *Persistence) RestoreSkillState(ctx context.Context, c *player.Character) error {
	if p == nil || !p.storeSkillCooltime.Load() || c == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	if p.store == nil {
		return nil
	}
	rows, err := p.store.ListByCharacter(ctx, c.ID, classIndex)
	if err != nil {
		return fmt.Errorf("restore skill state for character %d: %w", c.ID, err)
	}
	// The rows are filtered on p.now; the timers kept carry their stored
	// expiry and are checked on c's queue clock from then on, and the
	// staged effects run from c's queue clock instant (stageSkillState).
	p.stageSkillState(c, rows, p.currentTime())
	if _, err := p.store.DeleteByCharacter(ctx, c.ID, classIndex); err != nil {
		return fmt.Errorf("clear restored skill state for character %d: %w", c.ID, err)
	}
	return nil
}

// stageSkillState restores rows' reuse timers onto c and stages their
// effects for ReplayEffects, dropping what has run out by now. The staged
// effects' schedules run from this instant on c's clock, as an effect
// restored straight onto the character's list would.
func (p *Persistence) stageSkillState(c *player.Character, rows []effect.SaveRow, now time.Time) {
	plan := effect.BuildRestorePlan(rows, now.UnixMilli(), p.lookup)
	restoredAt := c.Now()
	for _, reuse := range plan.Reuse {
		def, ok := p.definition(reuse.Skill)
		if !ok {
			continue
		}
		c.RestoreSkillReuse(reuse.Skill, cast.ReuseKey(def), time.Duration(reuse.Delay)*time.Millisecond, time.UnixMilli(reuse.ExpiresAt))
	}
	for _, eff := range plan.Effects {
		def, ok := p.definition(eff.Skill)
		if !ok {
			continue
		}
		c.RestoreSkillEffect(eff, cast.ReuseKey(def), restoredAt)
	}
}

// ClassSkills is what one class index keeps of a character's skills: the
// skills it learned and the effects and reuse timers saved when it was
// last left.
type ClassSkills struct {
	levels player.SkillLevels
	saved  []effect.SaveRow
}

// LoadClassSkills reads charID's learned skills and saved skill state for
// classIndex, consuming the saved state: its rows are deleted once read,
// as a restore does. It runs off the character's queue; ApplyClassSkills
// puts the result in place on it.
func (p *Persistence) LoadClassSkills(ctx context.Context, charID, classIndex int32) (ClassSkills, error) {
	var cs ClassSkills
	if p == nil {
		return cs, nil
	}
	if p.levels != nil {
		levels, err := p.levels.ListKnownSkills(ctx, charID, classIndex)
		if err != nil {
			return cs, fmt.Errorf("load known skills for character %d class %d: %w", charID, classIndex, err)
		}
		cs.levels = levels
	}
	if !p.storeSkillCooltime.Load() || p.store == nil {
		return cs, nil
	}
	rows, err := p.store.ListByCharacter(ctx, charID, classIndex)
	if err != nil {
		return cs, fmt.Errorf("load skill state for character %d class %d: %w", charID, classIndex, err)
	}
	cs.saved = rows
	if _, err := p.store.DeleteByCharacter(ctx, charID, classIndex); err != nil {
		return cs, fmt.Errorf("clear loaded skill state for character %d class %d: %w", charID, classIndex, err)
	}
	return cs, nil
}

// ApplyClassSkills gives c the learned skills cs holds, with their passive
// stats. Call it on c's queue.
func (p *Persistence) ApplyClassSkills(c *player.Character, cs ClassSkills) error {
	if p == nil || c == nil {
		return nil
	}
	return p.applyKnownSkills(c, cs.levels)
}

// StageClassSkillState restores the reuse timers cs saved onto c and
// stages its effects for ReplayEffects, dropping what has run out. Call it
// on c's queue.
func (p *Persistence) StageClassSkillState(c *player.Character, cs ClassSkills) {
	if p == nil || c == nil {
		return
	}
	p.stageSkillState(c, cs.saved, c.Now())
}

// RemoveAllSkills takes every skill c knows away, with its passive stats,
// without touching character_skills.
func (p *Persistence) RemoveAllSkills(c *player.Character) {
	if p == nil || c == nil {
		return
	}
	for id := range c.SkillLevels() {
		// A removal attaches nothing, so it cannot fail.
		_ = p.setKnownSkill(c, id, 0, false)
	}
}

// ReplayEffects reinstates every effect Restore recorded into c's restore
// registry (via RestoreSkillEffect) onto c's live effect list, at the tick
// count and elapsed time it had at logout, run on from its restore instant.
// Call once c.EffectList() is attached, after Restore itself: Restore
// records restored effects into the registry only, so a character not yet
// in the world sends nothing for them — this replay is what actually fires
// their OnStart, schedules their ticks, and surfaces their icons, mirroring
// Player.restoreEffects()'s template.getEffect(this, this, skill) ->
// setCount/setTime -> scheduleEffect() chain. An effect whose ticks run an
// action runs those of the ticks due between the restore and the replay, in
// order, on c; one that ran out in between is not reinstated. A session
// that ends before EnterWorld replays them too, ahead of its saves.
func (p *Persistence) ReplayEffects(c *player.Character) {
	if p == nil || c == nil {
		return
	}
	list := c.EffectList()
	if list == nil {
		return
	}
	for _, eff := range c.ActiveSkillEffects() {
		def, ok := p.definition(eff.Skill)
		if !ok {
			continue
		}
		effect.ApplyRestored(list, c, c, effect.SkillFromDefinition(def), def.Effects, eff.Count, eff.Time, eff.RestoredAt)
	}
	// The registry's only purpose is staging Restore's effects until the live
	// effect list exists to receive them; Save now reads that live list
	// directly (see liveActiveEffects), so a stale, already-replayed entry
	// left behind here would never be read again — clear it so it can't
	// linger as dead state.
	c.ClearActiveSkillEffects()
}

// SetKnownSkill records one learned skill on the character and, when the
// backing store can write character_skills, persists it first. When the
// skill is passive, its stat functions are (re)attached to the character's
// live stat calculators so the bonus takes effect immediately; a prior
// level's functions are dropped first so relearning at a new level doesn't
// stack.
func (p *Persistence) SetKnownSkill(c *player.Character, skillID, level int) error {
	return p.setKnownSkill(c, skillID, level, true)
}

// ApplyTransientPassiveSkill replaces a skill's passive stat functions
// without adding it to the character's learned-skill state or persistence.
func (p *Persistence) ApplyTransientPassiveSkill(c *player.Character, skillID, oldLevel, level int) error {
	if c == nil {
		return nil
	}
	if oldLevel > 0 {
		c.RemoveStatsByOwner(effect.ModOwnerSkill(modelskill.Ref{ID: modelskill.ID(skillID), Level: oldLevel}))
	}
	if level <= 0 {
		return nil
	}
	def, ok := p.definition(modelskill.Ref{ID: modelskill.ID(skillID), Level: level})
	if !ok || def.Activation != modelskill.ActivationPassive {
		return nil
	}
	fns, err := effect.PassiveFuncs(def)
	if err != nil {
		return fmt.Errorf("apply transient passive stats for character %d skill %d level %d: %w", c.ID, skillID, level, err)
	}
	c.AddStatFuncs(fns)
	return nil
}

// setKnownSkill is SetKnownSkill with control over whether the change
// reaches character_skills. A skill the server hands out purely from the
// character's level is re-derived on every level change, so it is held in
// memory only: persisting it would leave a row behind that a later level
// loss has to clean up, and no such row is expected in the table either.
func (p *Persistence) setKnownSkill(c *player.Character, skillID, level int, persist bool) error {
	if c == nil {
		return nil
	}
	oldLevel := c.SkillLevel(skillID)
	c.SetSkillLevel(skillID, level)
	if persist {
		p.persistKnownSkill(c, skillID, level)
	}
	if oldLevel > 0 {
		c.RemoveStatsByOwner(effect.ModOwnerSkill(modelskill.Ref{ID: modelskill.ID(skillID), Level: oldLevel}))
	}
	if level <= 0 {
		return nil
	}
	def, ok := p.definition(modelskill.Ref{ID: modelskill.ID(skillID), Level: level})
	if !ok || def.Activation != modelskill.ActivationPassive {
		return nil
	}
	fns, err := effect.PassiveFuncs(def)
	if err != nil {
		return fmt.Errorf("apply passive stats for character %d skill %d level %d: %w", c.ID, skillID, level, err)
	}
	c.AddStatFuncs(fns)
	return nil
}

// knownSkillWriteTimeout bounds one character_skills write queued off an
// actor queue.
const knownSkillWriteTimeout = 2 * time.Second

// persistKnownSkill queues one learned skill level's write through to
// character_skills, on the character's persistence lane. A non-positive level
// is a removal, so it deletes the row rather than storing a level of 0, which
// would restore as a known skill the character does not have.
//
// The write follows the in-memory change and cannot undo it: the skill
// enters the character's map and its stat functions attach before it is
// stored, and a failed insert is only logged. A queued write must not decide
// whether the character learned the skill either, so a failure is logged
// here too.
func (p *Persistence) persistKnownSkill(c *player.Character, skillID, level int) {
	if p == nil || p.levels == nil {
		return
	}
	charID, classIndex := c.ID, c.SkillSaveClassIndex()
	var write func(context.Context) error
	switch {
	case level <= 0:
		deleter, ok := p.levels.(skillLevelDeleter)
		if !ok {
			return
		}
		write = func(ctx context.Context) error {
			if err := deleter.DeleteKnownSkill(ctx, charID, classIndex, skillID); err != nil {
				return fmt.Errorf("delete known skill for character %d: %w", charID, err)
			}
			return nil
		}
	default:
		writer, ok := p.levels.(skillLevelWriter)
		if !ok {
			return
		}
		write = func(ctx context.Context) error {
			if err := writer.SetKnownSkill(ctx, charID, classIndex, skillID, level); err != nil {
				return fmt.Errorf("set known skill for character %d: %w", charID, err)
			}
			return nil
		}
	}
	p.worker.Enqueue(charID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), knownSkillWriteTimeout)
		defer cancel()
		if err := write(ctx); err != nil {
			p.log.Error().Err(err).Int32("char_id", charID).Int("skill_id", skillID).Msg("persist known skill")
		}
	})
}

// EquipItemStats attaches the stat functions inst's template contributes
// while equipped and grants its item skills, mirroring the equip listeners in
// their order: the item's equip modifiers owned by the instance first, then
// the skills of the armor set inst completes (see armorSetGrants), then an
// augmented weapon's augmentation (see EquipItemStatsReporting), then —
// unless inst is a weapon above the character's Expertise — a weapon's +4
// enchant skill while inst is at +4 or higher, then every
// item.Template.AttachedSkills entry (any activation), each added to c's
// known-skill set the way learning it would, a passive one attaching its stat
// functions owned by the skill. Each attach reports its own stat change. Call
// once per instance, right after it becomes equipped. skillsChanged and
// timersChanged report whether the caller must resend SkillList and
// SkillCoolTime respectively.
func (p *Persistence) EquipItemStats(c *player.Character, inst *item.Instance, tmpl *item.Template) (skillsChanged, timersChanged bool, err error) {
	return p.EquipItemStatsReporting(c, inst, tmpl, nil)
}

// SkillChange reports what one equip or unequip stage — an armor set's
// skill grant or removal, or an augmentation's — changed beyond its stat
// bonuses: whether a skill was granted or removed, so SkillList must be
// resent, and whether a skill came back still waiting out its reuse, so
// SkillCoolTime must be sent too.
type SkillChange struct {
	SkillsChanged bool
	TimersChanged bool
}

// EquipItemStatsReporting is EquipItemStats with formal wear's skill list
// refresh, each armor set grant and the augmentation of an augmented weapon
// reported as its own stage. Formal wear and the armor set grants follow the
// item's own functions, each one answered by its own skill list, one per
// grant. The augmentation follows them ahead of
// the item's skills, applied ahead of the grade penalty check:
// its stat bonuses attach without a stat report and its skill, if any, is
// granted. stage, when not nil, is told what each of those changed at that
// moment, for a caller that answers it with its own packets; a nil stage
// folds them into the result instead.
func (p *Persistence) EquipItemStatsReporting(c *player.Character, inst *item.Instance, tmpl *item.Template, stage func(SkillChange)) (skillsChanged, timersChanged bool, err error) {
	if p == nil || c == nil || inst == nil || tmpl == nil {
		return false, false, nil
	}
	aug, augmentedWeapon := inst.AugmentationValue()
	augmentedWeapon = augmentedWeapon && tmpl.Weapon != nil
	var augMods []effect.Mod
	var augGrant *itemSkillGrant
	if augmentedWeapon {
		augMods, augGrant, err = p.augmentationGrant(inst, aug)
		if err != nil {
			return false, false, fmt.Errorf("apply augmentation for character %d item %d: %w", c.ID, inst.ObjectID, err)
		}
	}
	owner := effect.ItemOwner{Inst: inst, Tmpl: tmpl}
	modFns, err := effect.ItemModifierFuncs(owner)
	if err != nil {
		return false, false, fmt.Errorf("apply equip modifiers for character %d item %d: %w", c.ID, inst.ObjectID, err)
	}
	// A weapon whose crystal grade the character's Expertise doesn't yet
	// allow skips its whole item-skill grant (the grade penalty check
	// returns before the +4 skill and the item_skill loop
	// run), so none of its granted skills apply until Expertise catches up.
	var grants []itemSkillGrant
	if tmpl.Weapon == nil || c.WeaponSkillsAllowed(tmpl.Crystal) {
		if inst.Snapshot().EnchantLevel >= item.Enchant4SkillLevel {
			g, ok, err := p.enchant4SkillGrant(tmpl)
			if err != nil {
				return false, false, fmt.Errorf("apply equip passives for character %d item %d: %w", c.ID, inst.ObjectID, err)
			}
			if ok {
				grants = append(grants, g)
			}
		}
		attached, err := p.itemSkillGrants(tmpl)
		if err != nil {
			return false, false, fmt.Errorf("apply equip passives for character %d item %d: %w", c.ID, inst.ObjectID, err)
		}
		grants = append(grants, attached...)
	}
	setStages, err := p.armorSetGrants(c, inst, tmpl)
	if err != nil {
		return false, false, fmt.Errorf("apply armor set for character %d item %d: %w", c.ID, inst.ObjectID, err)
	}
	report := func(change SkillChange) {
		if stage != nil {
			stage(change)
			return
		}
		skillsChanged = skillsChanged || change.SkillsChanged
		timersChanged = timersChanged || change.TimersChanged
	}
	c.AddStatFuncs(modFns)
	// Formal wear answers its own equip with a skill list, every entry
	// greyed out by it, ahead of its item skills. Only a caller that
	// reports each stage hears of it: one that folds them refreshes the
	// whole paperdoll and sends no listener packets.
	if tmpl.Slot == item.SlotAllDress && stage != nil {
		stage(SkillChange{SkillsChanged: true})
	}
	for _, group := range setStages {
		p.grantItemSkills(c, group)
		report(SkillChange{SkillsChanged: true})
	}
	if augmentedWeapon {
		report(p.applyAugmentation(c, augMods, augGrant))
	}
	itemSkills, itemTimers := p.grantItemSkills(c, grants)
	return skillsChanged || itemSkills, timersChanged || itemTimers, nil
}

// augmentationGrant resolves what aug, carried by inst, applies: its stat
// bonuses owned by the augmentation, and its skill when it names a loaded
// one.
func (p *Persistence) augmentationGrant(inst *item.Instance, aug item.Augmentation) ([]effect.Mod, *itemSkillGrant, error) {
	var mods []effect.Mod
	if p.augments != nil {
		for _, b := range p.augments.Bonuses(aug.Attributes) {
			s, err := stat.ByName(b.Stat)
			if err != nil {
				return nil, nil, err
			}
			mods = append(mods, effect.Mod{Stat: s, Op: effect.OpAdd, Value: float64(b.Value), Owner: effect.ModOwnerAugmentation(inst)})
		}
	}
	if aug.SkillID == 0 {
		return mods, nil, nil
	}
	def, ok := p.definition(modelskill.Ref{ID: modelskill.ID(aug.SkillID), Level: int(aug.SkillLevel)})
	if !ok {
		return mods, nil, nil
	}
	g := itemSkillGrant{def: def, noEquipDelay: true}
	if def.Activation == modelskill.ActivationPassive {
		fns, err := effect.PassiveFuncs(def)
		if err != nil {
			return nil, nil, fmt.Errorf("skill %d level %d: %w", aug.SkillID, aug.SkillLevel, err)
		}
		g.fns = fns
	}
	return mods, &g, nil
}

// applyAugmentation attaches an augmentation's stat bonuses, which report
// no stat change of their own, then grants its skill. An active skill whose
// reuse is still running when it comes back is disabled until it ends.
func (p *Persistence) applyAugmentation(c *player.Character, mods []effect.Mod, grant *itemSkillGrant) SkillChange {
	c.AttachStatFuncs(mods)
	if grant == nil {
		return SkillChange{}
	}
	p.grantItemSkills(c, []itemSkillGrant{*grant})
	change := SkillChange{SkillsChanged: true}
	if grant.def.Activation == modelskill.ActivationActive {
		change.TimersChanged = c.RedisableSkillReuse(cast.ReuseKey(grant.def))
	}
	return change
}

// removeAugmentation detaches the stat bonuses of the augmentation inst
// carries, then takes its skill away when it names a loaded one.
func (p *Persistence) removeAugmentation(c *player.Character, inst *item.Instance) SkillChange {
	c.RemoveStatsByOwner(effect.ModOwnerAugmentation(inst))
	aug, ok := inst.AugmentationValue()
	if !ok || aug.SkillID == 0 || !p.HasDefinition(modelskill.Ref{ID: modelskill.ID(aug.SkillID), Level: int(aug.SkillLevel)}) {
		return SkillChange{}
	}
	removeItemSkill(c, int(aug.SkillID))
	return SkillChange{SkillsChanged: true}
}

// armorSetCommonSkillID is the skill every complete armor set grants
// alongside its own set skill.
const armorSetCommonSkillID = 3006

// armorSetGrants resolves the skills the worn armor set gains from inst,
// newly equipped, one group per skill list sent: the common
// set skill and the set skill when inst is one of the worn chest's set
// pieces and completes the set, then the shield skill when the set's shield
// is held, then the +6 skill when every piece is at +6 or higher — or, when
// inst is the set's shield and the set is complete, the shield skill alone.
// A group whose skill is not loaded is left out. Formal wear grants nothing
// here. Everything is resolved before any is granted, so a malformed passive
// leaves the character untouched.
func (p *Persistence) armorSetGrants(c *player.Character, inst *item.Instance, tmpl *item.Template) ([][]itemSkillGrant, error) {
	if p.armorSets == nil || tmpl.Slot == item.SlotAllDress {
		return nil, nil
	}
	inv := c.Inventory()
	if inv == nil {
		return nil, nil
	}
	set, ok := p.armorSets.Worn(inv)
	if !ok {
		return nil, nil
	}
	var stages [][]itemSkillGrant
	// add appends one group led by lead, which must be loaded for the group
	// to apply; extra skills ride along only when loaded themselves.
	add := func(lead int32, extra ...int32) error {
		g, ok, err := p.armorSetSkillGrant(lead)
		if err != nil || !ok {
			return err
		}
		group := make([]itemSkillGrant, 0, 1+len(extra))
		for _, id := range extra {
			e, ok, err := p.armorSetSkillGrant(id)
			if err != nil {
				return err
			}
			if ok {
				group = append(group, e)
			}
		}
		stages = append(stages, append(group, g))
		return nil
	}
	slot := armorSetSlot(tmpl)
	switch {
	case set.ContainsItem(slot, inst.TemplateID):
		if !set.ContainsAll(inv) {
			return nil, nil
		}
		if err := add(set.SkillID, armorSetCommonSkillID); err != nil {
			return nil, err
		}
		if set.WearsShield(inv) {
			if err := add(set.ShieldSkillID); err != nil {
				return nil, err
			}
		}
		if set.Enchanted6(inv) {
			if err := add(set.Enchant6Skill); err != nil {
				return nil, err
			}
		}
	case set.IsShield(inst.TemplateID) && set.ContainsAll(inv):
		if err := add(set.ShieldSkillID); err != nil {
			return nil, err
		}
	}
	return stages, nil
}

// unequipArmorSet removes the skills inst, just unequipped, took from its
// armor set: a set chest takes its set's common, set, shield and +6 skills;
// another piece of the set of the chest worn at that step (wornChestID, 0
// for none) takes the same; that set's shield takes the shield skill. It
// reports whether inst belonged to a set that way, which is when the caller
// resends SkillList — even when none of those skills was known.
func (p *Persistence) unequipArmorSet(c *player.Character, wornChestID int32, inst *item.Instance, tmpl *item.Template) bool {
	if p == nil || p.armorSets == nil || tmpl.Slot == item.SlotAllDress {
		return false
	}
	slot := armorSetSlot(tmpl)
	var set armorset.Set
	var ok bool
	if slot == itemcontainer.Chest {
		set, ok = p.armorSets.FindByChest(inst.TemplateID)
	} else if wornChestID != 0 {
		set, ok = p.armorSets.FindByChest(wornChestID)
	}
	if !ok {
		return false
	}
	var setSkill, shieldSkill, enchant6Skill int32
	switch {
	case slot == itemcontainer.Chest || set.ContainsItem(slot, inst.TemplateID):
		setSkill, shieldSkill, enchant6Skill = set.SkillID, set.ShieldSkillID, set.Enchant6Skill
	case set.IsShield(inst.TemplateID):
		shieldSkill = set.ShieldSkillID
	default:
		return false
	}
	if setSkill != 0 {
		removeItemSkill(c, armorSetCommonSkillID)
		removeItemSkill(c, int(setSkill))
	}
	if shieldSkill != 0 {
		removeItemSkill(c, int(shieldSkill))
	}
	if enchant6Skill != 0 {
		removeItemSkill(c, int(enchant6Skill))
	}
	return true
}

// armorSetSlot is the paperdoll position tmpl occupies when worn, or -1 for
// a slot that resolves to none, which holds no set piece.
func armorSetSlot(tmpl *item.Template) int {
	if slot, ok := tmpl.Slot.PaperdollIndex(); ok {
		return slot
	}
	return -1
}

// armorSetSkillGrant resolves level 1 of an armor set skill. ok is false
// when it is not loaded.
func (p *Persistence) armorSetSkillGrant(id int32) (itemSkillGrant, bool, error) {
	g, ok, err := p.listenerSkillGrant(modelskill.Ref{ID: modelskill.ID(id), Level: 1})
	if err != nil {
		return itemSkillGrant{}, false, fmt.Errorf("armor set skill %d: %w", id, err)
	}
	return g, ok, nil
}

// listenerSkillGrant resolves ref as a grant that arms no equip delay,
// with its stat functions when passive. ok is false when ref is not
// loaded.
func (p *Persistence) listenerSkillGrant(ref modelskill.Ref) (g itemSkillGrant, ok bool, err error) {
	def, ok := p.definition(ref)
	if !ok {
		return itemSkillGrant{}, false, nil
	}
	g = itemSkillGrant{def: def, noEquipDelay: true}
	if def.Activation == modelskill.ActivationPassive {
		if g.fns, err = effect.PassiveFuncs(def); err != nil {
			return itemSkillGrant{}, false, err
		}
	}
	return g, true, nil
}

// GrantArmorSetSkill adds level 1 of skillID, an armor set's +6 skill, to
// c's known-skill set, as a scroll of enchant taking a worn piece of a
// fully +6 set to +6 does. It reports whether the skill is loaded, which
// is when the caller resends SkillList.
func (p *Persistence) GrantArmorSetSkill(c *player.Character, skillID int32) (bool, error) {
	if p == nil || c == nil {
		return false, nil
	}
	g, ok, err := p.armorSetSkillGrant(skillID)
	if err != nil || !ok {
		return false, err
	}
	p.grantItemSkills(c, []itemSkillGrant{g})
	return true, nil
}

// RevokeArmorSetSkill drops skillID, an armor set's +6 skill, from c's
// known-skill set with the stat functions of the level c knows it at.
func (p *Persistence) RevokeArmorSetSkill(c *player.Character, skillID int32) {
	if p == nil || c == nil {
		return
	}
	removeItemSkill(c, int(skillID))
}

// enchant4SkillGrant resolves tmpl's +4 enchant skill. ok is false for a
// non-weapon, a weapon without one, or one naming no loaded skill.
func (p *Persistence) enchant4SkillGrant(tmpl *item.Template) (g itemSkillGrant, ok bool, err error) {
	if tmpl == nil || tmpl.Weapon == nil || tmpl.Weapon.Enchant4Skill == nil {
		return itemSkillGrant{}, false, nil
	}
	ref := *tmpl.Weapon.Enchant4Skill
	g, ok, err = p.listenerSkillGrant(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
	if err != nil {
		return itemSkillGrant{}, false, fmt.Errorf("item %d enchant skill %d level %d: %w", tmpl.ID, ref.ID, ref.Level, err)
	}
	return g, ok, nil
}

// GrantEnchant4Skill adds tmpl's +4 enchant skill to c's known-skill set, as
// a scroll of enchant taking an equipped weapon to +4 does. It reports
// whether tmpl has a loaded +4 skill, which is when the caller resends
// SkillList. No Expertise check applies here: the enchant path grants it
// whatever the weapon's grade.
func (p *Persistence) GrantEnchant4Skill(c *player.Character, tmpl *item.Template) (bool, error) {
	if p == nil || c == nil {
		return false, nil
	}
	g, ok, err := p.enchant4SkillGrant(tmpl)
	if err != nil || !ok {
		return false, err
	}
	p.grantItemSkills(c, []itemSkillGrant{g})
	return true, nil
}

// RevokeEnchant4Skill drops tmpl's +4 enchant skill from c's known-skill
// set with the stat functions of the level c knows it at. It reports whether
// tmpl has a loaded +4 skill, which is when the caller resends SkillList —
// even when c did not know it.
func (p *Persistence) RevokeEnchant4Skill(c *player.Character, tmpl *item.Template) bool {
	if p == nil || c == nil {
		return false
	}
	g, ok, err := p.enchant4SkillGrant(tmpl)
	if err != nil || !ok {
		return ok
	}
	removeItemSkill(c, int(g.def.ID))
	return true
}

// removeItemSkill drops skillID from c's known-skill set with the stat
// functions of the level c knows it at. It reports whether c knew it.
func removeItemSkill(c *player.Character, skillID int) bool {
	level := c.SkillLevel(skillID)
	if level <= 0 {
		return false
	}
	c.SetSkillLevel(skillID, 0)
	c.RemoveStatsByOwner(effect.ModOwnerSkill(modelskill.Ref{ID: modelskill.ID(skillID), Level: level}))
	return true
}

// itemSkillGrant is one loaded tmpl.AttachedSkills entry and, for a passive
// one, the stat functions it attaches, owned by the skill.
type itemSkillGrant struct {
	def modelskill.Definition
	fns []effect.Mod
	// noEquipDelay skips the equip-delay reuse timer an ACTIVE item skill
	// arms: the +4 enchant skill is added without one.
	noEquipDelay bool
}

// itemSkillGrants resolves tmpl's attached skills before any is granted, so
// a malformed passive leaves the character untouched. An entry naming no
// loaded skill is skipped.
func (p *Persistence) itemSkillGrants(tmpl *item.Template) ([]itemSkillGrant, error) {
	var grants []itemSkillGrant
	for _, ref := range tmpl.AttachedSkills {
		def, ok := p.definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
		if !ok {
			continue
		}
		g := itemSkillGrant{def: def}
		if def.Activation == modelskill.ActivationPassive {
			fns, err := effect.PassiveFuncs(def)
			if err != nil {
				return nil, fmt.Errorf("item %d passive skill %d level %d: %w", tmpl.ID, ref.ID, ref.Level, err)
			}
			g.fns = fns
		}
		grants = append(grants, g)
	}
	return grants, nil
}

// grantItemSkills adds each granted skill to c's known-skill set without
// persisting it, as learning it in memory would: a skill c already knows at
// that level changes nothing (so two equipped items granting it
// attach its passive stat functions once); at another level the known
// level's stat functions are dropped before the granted level's attach. An
// ACTIVE entry with a positive equip delay and no already-armed reuse timer
// for its reuse key gets one armed, matching
// ItemPassiveSkillsListener.onEquip:58-72.
func (p *Persistence) grantItemSkills(c *player.Character, grants []itemSkillGrant) (skillsChanged, timersChanged bool) {
	for _, g := range grants {
		def := g.def
		skillsChanged = true
		if old := c.SkillLevel(int(def.ID)); old != def.Level {
			c.SetSkillLevel(int(def.ID), def.Level)
			if old > 0 {
				c.RemoveStatsByOwner(effect.ModOwnerSkill(modelskill.Ref{ID: def.ID, Level: old}))
			}
			c.AddStatFuncs(g.fns)
		}
		if def.Activation != modelskill.ActivationActive || g.noEquipDelay {
			continue
		}
		key := cast.ReuseKey(def)
		if def.EquipDelay > 0 && !c.HasSkillReuse(key) {
			c.AddSkillReuse(modelskill.Ref{ID: def.ID, Level: def.Level}, key, time.Duration(def.EquipDelay)*time.Millisecond)
		}
		timersChanged = true
	}
	return skillsChanged, timersChanged
}

// UnequipItemStats removes every stat function inst contributed via
// EquipItemStats, mirroring the unequip listeners in their order: the
// instance's own functions first, then the skills of the worn armor set inst
// belongs to (see unequipArmorSet), then an augmented weapon's augmentation
// (see UnequipItemStatsReporting), then — for a weapon at +4 or higher — its
// +4 enchant skill, then — unless inv still has another equipped item
// sharing tmpl's id — every tmpl.AttachedSkills entry leaves c's known-skill
// set with the stat functions of the level c knows it at. Each removal
// reports its own stat change, so a passive-granting item sends one refresh
// for the item and one per skill. tmpl must be the same template instance
// EquipItemStats was called with, so the owner identity used to attach the
// functions matches the one used to remove them. skillsChanged reports
// whether the caller must resend SkillList: a loaded +4 skill asks for it at
// +4 or higher even when the Expertise gate kept it from being granted. No
// reuse timer armed by the equip-delay grant is cleared.
func (p *Persistence) UnequipItemStats(c *player.Character, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) (skillsChanged bool) {
	var step WornStep
	if inv != nil {
		step.Items = inv.PaperdollItems()
		if chest := inv.ItemAt(itemcontainer.Chest); chest != nil {
			step.ChestID = chest.TemplateID
		}
	}
	return p.UnequipItemStatsReporting(c, step, inst, tmpl, nil)
}

// WornStep is the paperdoll one unequip step of an equip change sees, once
// the item it takes off has left it: an equip change that moves several
// slots runs the listeners of each slot in turn, so a piece cleared ahead of
// the chest still sees the old chest.
type WornStep struct {
	// Items are the instances still worn.
	Items []*item.Instance
	// ChestID is the template id in the chest slot, 0 when it is empty.
	ChestID int32
}

// UnequipItemStatsReporting is UnequipItemStats against the paperdoll worn
// at that step, with formal wear's skill list refresh, the armor set skill
// removal and the augmentation of an augmented weapon reported as their own
// stages, between the item's own functions and its +4 skill, in that order
// as the armor set and skill listeners run: formal wear and the armor set's
// skills each leave with one skill list, then the augmentation's stat
// bonuses detach with their own stat report and its skill, if any, leaves.
// stage, when not nil, is told what each of those changed at that moment; a
// nil stage folds the armor set and augmentation into the result instead,
// and leaves formal wear out.
func (p *Persistence) UnequipItemStatsReporting(c *player.Character, worn WornStep, inst *item.Instance, tmpl *item.Template, stage func(SkillChange)) (skillsChanged bool) {
	if c == nil || inst == nil {
		return false
	}
	c.RemoveStatsByOwner(effect.ModOwnerItem(effect.ItemOwner{Inst: inst, Tmpl: tmpl}))
	if tmpl == nil {
		return false
	}
	report := func(change SkillChange) {
		if stage != nil {
			stage(change)
			return
		}
		skillsChanged = skillsChanged || change.SkillsChanged
	}
	if tmpl.Slot == item.SlotAllDress && stage != nil {
		stage(SkillChange{SkillsChanged: true})
	}
	if p.unequipArmorSet(c, worn.ChestID, inst, tmpl) {
		report(SkillChange{SkillsChanged: true})
	}
	if tmpl.Weapon != nil && inst.Augmented() {
		report(p.removeAugmentation(c, inst))
	}
	if inst.Snapshot().EnchantLevel >= item.Enchant4SkillLevel && p.RevokeEnchant4Skill(c, tmpl) {
		skillsChanged = true
	}
	for _, other := range worn.Items {
		if other != inst && other.TemplateID == tmpl.ID {
			return skillsChanged
		}
	}
	for _, ref := range tmpl.AttachedSkills {
		if removeItemSkill(c, int(ref.ID)) {
			skillsChanged = true
		}
	}
	return skillsChanged
}

// RestoreEquippedItemStats attaches the stat functions every item currently
// equipped in inv contributes, for reinstating a relogging character's
// equip-granted passives and modifiers alongside the learned-skill restore
// Restore already performs.
func (p *Persistence) RestoreEquippedItemStats(c *player.Character, inv *itemcontainer.Inventory) error {
	if c == nil || inv == nil {
		return nil
	}
	for _, inst := range inv.PaperdollItems() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			continue
		}
		if _, _, err := p.EquipItemStats(c, inst, tmpl); err != nil {
			return err
		}
	}
	return nil
}

// RefreshEquippedItemStats reapplies equipped item modifiers and passives,
// and item-granted skills, after a state gate changes which of them may be
// active (e.g. an Expertise level change). skillsChanged and timersChanged
// report whether the caller must resend SkillList and SkillCoolTime.
func (p *Persistence) RefreshEquippedItemStats(c *player.Character, inv *itemcontainer.Inventory) (skillsChanged, timersChanged bool, err error) {
	if c == nil || inv == nil {
		return false, false, nil
	}
	for _, inst := range inv.PaperdollItems() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			continue
		}
		if p.UnequipItemStats(c, inv, inst, tmpl) {
			skillsChanged = true
		}
		equipSkills, equipTimers, err := p.EquipItemStats(c, inst, tmpl)
		if err != nil {
			return false, false, err
		}
		skillsChanged = skillsChanged || equipSkills
		timersChanged = timersChanged || equipTimers
	}
	return skillsChanged, timersChanged, nil
}

// Definition returns a loaded skill definition.
func (p *Persistence) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	return p.definition(ref)
}

// HasDefinition reports whether a skill definition is loaded.
func (p *Persistence) HasDefinition(ref modelskill.Ref) bool {
	_, ok := p.definition(ref)
	return ok
}

// MaxLevel returns the highest regular level loaded for id.
func (p *Persistence) MaxLevel(id modelskill.ID) int {
	if p == nil || p.skills == nil {
		return 0
	}
	return p.skills.MaxLevel(id)
}

func (p *Persistence) restoreKnownSkills(ctx context.Context, c *player.Character, classIndex int32) error {
	if p.levels == nil {
		return nil
	}
	levels, err := p.levels.ListKnownSkills(ctx, c.ID, classIndex)
	if err != nil {
		return fmt.Errorf("restore known skills for character %d: %w", c.ID, err)
	}
	return p.applyKnownSkills(c, levels)
}

// applyKnownSkills gives c each learned skill of levels, attaching a
// passive one's stat functions. A skill whose definition is no longer
// loaded is skipped.
func (p *Persistence) applyKnownSkills(c *player.Character, levels player.SkillLevels) error {
	for id, level := range levels {
		if level <= 0 {
			continue
		}
		ref := modelskill.Ref{ID: modelskill.ID(id), Level: level}
		def, ok := p.definition(ref)
		if p.skills != nil && !ok {
			continue
		}
		c.SetSkillLevel(id, level)
		if !ok || def.Activation != modelskill.ActivationPassive {
			continue
		}
		fns, err := effect.PassiveFuncs(def)
		if err != nil {
			return fmt.Errorf("restore passive stats for character %d skill %d level %d: %w", c.ID, id, level, err)
		}
		c.AddStatFuncs(fns)
	}
	return nil
}

func (p *Persistence) lookup(ref modelskill.Ref) (bool, bool) {
	def, ok := p.definition(ref)
	if !ok {
		return false, false
	}
	// Self-targeted templates apply at cast time and are not reinstated on
	// relog; only ordinary effect templates count as restorable.
	return true, len(def.Effects) > 0
}

func (p *Persistence) definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	if p == nil || p.skills == nil {
		return modelskill.Definition{}, false
	}
	return p.skills.Get(ref.ID, ref.Level)
}

func (p *Persistence) currentTime() time.Time {
	if p != nil && p.now != nil {
		return p.now()
	}
	return time.Now()
}

// StoreClassSkills writes levels to character_skills as charID's learned
// skills for classIndex. It runs off the character's queue.
func (p *Persistence) StoreClassSkills(ctx context.Context, charID, classIndex int32, levels player.SkillLevels) error {
	if p == nil {
		return nil
	}
	writer, ok := p.levels.(skillLevelWriter)
	if !ok {
		return nil
	}
	for id, level := range levels {
		if err := writer.SetKnownSkill(ctx, charID, classIndex, id, level); err != nil {
			return fmt.Errorf("store class %d skills for character %d: %w", classIndex, charID, err)
		}
	}
	return nil
}

// DuelState copies c's current active effects and pending reuse timers as
// a duel saves them when it begins, before c joins it: the duel's end puts
// them back (RestoreDuelState).
func (p *Persistence) DuelState(c *player.Character) SaveState {
	return p.snapshot(c)
}

// RestoreDuelState puts back on c the reuse timers st saved that are still
// running and the effects st saved, with the time they had left then. Call
// it on c's queue once c's effects are stopped; the rows the duel wrote are
// ClearSaved's to delete.
func (p *Persistence) RestoreDuelState(c *player.Character, st SaveState) {
	if p == nil || c == nil {
		return
	}
	p.stageSkillState(c, st.rows, c.Now())
	p.ReplayEffects(c)
}

// ClearSaved deletes the rows st's character saved for st's class.
func (p *Persistence) ClearSaved(ctx context.Context, st SaveState) error {
	if !st.ok {
		return nil
	}
	if _, err := p.store.DeleteByCharacter(ctx, st.charID, st.classIndex); err != nil {
		return fmt.Errorf("clear skill state for character %d: %w", st.charID, err)
	}
	return nil
}
