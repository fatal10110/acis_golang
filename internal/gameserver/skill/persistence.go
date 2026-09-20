package skill

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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

// SaveState copies c's current active effects and pending reuse timers.
func (p *Persistence) SaveState(c *player.Character) SaveState {
	if p == nil || !p.storeSkillCooltime.Load() || p.store == nil || c == nil {
		return SaveState{}
	}
	classIndex := c.SkillSaveClassIndex()
	rows := effect.BuildSaveRows(p.liveActiveEffects(c), c.SkillReuseTimers(p.currentTime()), classIndex)
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
// what gets saved, not a separate write-only registry. An effect whose skill
// definition no longer resolves is dropped, matching a stale datapack change.
func (p *Persistence) liveActiveEffects(c *player.Character) []effect.ActiveEffect {
	list := c.EffectList()
	if list == nil {
		return nil
	}
	now := p.currentTime()
	var out []effect.ActiveEffect
	for _, e := range list.All() {
		if !e.InUse() {
			continue
		}
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

// RestoreKnownSkills restores learned skills independently from effects and reuse timers.
func (p *Persistence) RestoreKnownSkills(ctx context.Context, c *player.Character) error {
	if p == nil || c == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	return p.restoreKnownSkills(ctx, c, classIndex)
}

// RestoreSkillState consumes persisted effects and reuse timers.
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
	plan := effect.BuildRestorePlan(rows, p.currentTime().UnixMilli(), p.lookup)
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
		c.RestoreSkillEffect(eff, cast.ReuseKey(def))
	}
	if _, err := p.store.DeleteByCharacter(ctx, c.ID, classIndex); err != nil {
		return fmt.Errorf("clear restored skill state for character %d: %w", c.ID, err)
	}
	return nil
}

// ReplayEffects reinstates every effect Restore recorded into c's restore
// registry (via RestoreSkillEffect) onto c's live effect list, at the tick
// count and elapsed time it had at logout. Call once c.EffectList() is
// attached, after Restore itself: Restore runs before the live player (and
// its effect list) exists, so it can only record restored effects into the
// registry — this replay is what actually fires their OnStart, schedules
// their ticks, and surfaces their icons, mirroring
// Player.restoreEffects()'s template.getEffect(this, this, skill) ->
// setCount/setTime -> scheduleEffect() chain.
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
		effect.ApplyRestored(list, c, c, effect.SkillFromDefinition(def), def.Effects, eff.Count, eff.Time)
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
// loss has to clean up, and the reference does not write one either.
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
// The write follows the in-memory change and cannot undo it: the reference
// puts the skill in the character's map, attaches its stat functions and only
// then calls storeSkill, which logs a failed insert and returns
// (Player.addSkill, Player.storeSkill). A queued write must not decide
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
// while equipped — item.Template.AttachedSkills passives and
// item.Template.Modifiers equip bonuses — to c's live stat calculators, and
// grants every item.Template.AttachedSkills entry (any activation) to c's
// known-skill set, mirroring ItemPassiveSkillsListener.onEquip. Call once
// per instance, right after it becomes equipped. skillsChanged and
// timersChanged report whether the caller must resend SkillList and
// SkillCoolTime respectively.
func (p *Persistence) EquipItemStats(c *player.Character, inst *item.Instance, tmpl *item.Template) (skillsChanged, timersChanged bool, err error) {
	if p == nil || c == nil || inst == nil || tmpl == nil {
		return false, false, nil
	}
	owner := effect.ItemOwner{Inst: inst, Tmpl: tmpl}
	modFns, err := effect.ItemModifierFuncs(owner)
	if err != nil {
		return false, false, fmt.Errorf("apply equip modifiers for character %d item %d: %w", c.ID, inst.ObjectID, err)
	}
	var passiveFns []effect.Mod
	// A weapon whose crystal grade the character's Expertise doesn't yet
	// allow skips its whole item_skill loop in the reference (the grade
	// penalty check returns before that loop runs), so neither its passive
	// stat funcs nor any of its granted skills apply until Expertise catches
	// up.
	if tmpl.Weapon == nil || c.WeaponSkillsAllowed(tmpl.Crystal) {
		passiveFns, err = effect.ItemPassiveFuncs(p.skills, owner)
		if err != nil {
			return false, false, fmt.Errorf("apply equip passives for character %d item %d: %w", c.ID, inst.ObjectID, err)
		}
		skillsChanged, timersChanged = p.grantItemSkills(c, tmpl)
	}
	c.AddStatFuncs(modFns)
	c.AddStatFuncs(passiveFns)
	return skillsChanged, timersChanged, nil
}

// grantItemSkills grants every tmpl.AttachedSkills entry to c's known-skill
// set, regardless of activation type, without persisting it and without
// attaching its stat functions (ItemPassiveFuncs already attaches a passive
// entry's stat functions, owned by the item instance). An ACTIVE entry with
// a positive equip delay and no already-armed reuse timer for its reuse key
// gets one armed, matching ItemPassiveSkillsListener.onEquip:58-72.
func (p *Persistence) grantItemSkills(c *player.Character, tmpl *item.Template) (skillsChanged, timersChanged bool) {
	for _, ref := range tmpl.AttachedSkills {
		def, ok := p.definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
		if !ok {
			continue
		}
		c.SetSkillLevel(int(ref.ID), int(ref.Level))
		skillsChanged = true
		if def.Activation != modelskill.ActivationActive {
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

// UnequipItemStats removes every stat function inst previously contributed
// via EquipItemStats, and revokes every tmpl.AttachedSkills entry from c's
// known-skill set unless inv still has another equipped item sharing tmpl's
// id, mirroring ItemPassiveSkillsListener.onUnequip. tmpl must be the same
// template instance EquipItemStats was called with, so the owner identity
// used to attach the functions matches the one used to remove them.
// skillsChanged reports whether the caller must resend SkillList; no reuse
// timer armed by the equip-delay grant is cleared, matching the reference.
func (p *Persistence) UnequipItemStats(c *player.Character, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) (skillsChanged bool) {
	if c == nil || inst == nil {
		return false
	}
	c.RemoveStatsByOwner(effect.ModOwnerItem(effect.ItemOwner{Inst: inst, Tmpl: tmpl}))
	if tmpl == nil || inv == nil {
		return false
	}
	for _, other := range inv.PaperdollItems() {
		if other == inst {
			continue
		}
		if other.TemplateID == tmpl.ID {
			return false
		}
	}
	for _, ref := range tmpl.AttachedSkills {
		if c.SkillLevel(int(ref.ID)) <= 0 {
			continue
		}
		c.SetSkillLevel(int(ref.ID), 0)
		skillsChanged = true
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
