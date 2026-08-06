package skill

import (
	"context"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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
}

// NewPersistence returns a lifecycle persistence component backed by store and
// the loaded skill table.
func NewPersistence(store skillSaveStore, skills *modelskill.Table, levels ...skillLevelStore) *Persistence {
	return NewPersistenceWithClock(store, skills, time.Now, levels...)
}

// NewPersistenceWithClock returns a lifecycle persistence component using now
// as its time source.
func NewPersistenceWithClock(store skillSaveStore, skills *modelskill.Table, now func() time.Time, levels ...skillLevelStore) *Persistence {
	p := &Persistence{store: store, skills: skills, now: now}
	if len(levels) > 0 {
		p.levels = levels[0]
	}
	return p
}

// Save replaces c's persisted skill state with its current active effects and
// pending reuse timers.
func (p *Persistence) Save(ctx context.Context, c *player.Character, includeEffects bool) error {
	if p == nil || p.store == nil || c == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	rows := effect.BuildSaveRows(c.ActiveSkillEffects(), c.SkillReuseTimers(p.currentTime()), classIndex, includeEffects)
	if err := p.store.Replace(ctx, c.ID, classIndex, rows); err != nil {
		return fmt.Errorf("save skill state for character %d: %w", c.ID, err)
	}
	return nil
}

// Restore consumes c's persisted skill state, reinstating pending reuse timers
// and effect rows whose skill definitions still exist.
func (p *Persistence) Restore(ctx context.Context, c *player.Character) error {
	if p == nil || c == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	if err := p.restoreKnownSkills(ctx, c, classIndex); err != nil {
		return err
	}
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
		templates := def.Effects
		if len(def.SelfEffects) > 0 {
			templates = append(append([]modelskill.EffectTemplate{}, templates...), def.SelfEffects...)
		}
		effect.ApplyRestored(list, c, c, effect.SkillFromDefinition(def), templates, eff.Count, eff.Time)
	}
}

// SetKnownSkill records one learned skill on the character and, when the
// backing store can write character_skills, persists it first. When the
// skill is passive, its stat functions are (re)attached to the character's
// live stat calculators so the bonus takes effect immediately; a prior
// level's functions are dropped first so relearning at a new level doesn't
// stack.
func (p *Persistence) SetKnownSkill(ctx context.Context, c *player.Character, skillID, level int) error {
	return p.setKnownSkill(ctx, c, skillID, level, true)
}

// setKnownSkill is SetKnownSkill with control over whether the change
// reaches character_skills. A skill the server hands out purely from the
// character's level is re-derived on every level change, so it is held in
// memory only: persisting it would leave a row behind that a later level
// loss has to clean up, and the reference does not write one either.
func (p *Persistence) setKnownSkill(ctx context.Context, c *player.Character, skillID, level int, persist bool) error {
	if c == nil {
		return nil
	}
	if persist {
		if err := p.persistKnownSkill(ctx, c, skillID, level); err != nil {
			return err
		}
	}
	oldLevel := c.SkillLevel(skillID)
	c.SetSkillLevel(skillID, level)
	if oldLevel > 0 {
		c.RemoveStatsByOwner(modelskill.Ref{ID: modelskill.ID(skillID), Level: oldLevel})
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

// persistKnownSkill writes one learned skill level through to
// character_skills. A non-positive level is a removal, so it deletes the row
// rather than storing a level of 0, which would restore as a known skill the
// character does not have.
func (p *Persistence) persistKnownSkill(ctx context.Context, c *player.Character, skillID, level int) error {
	if p == nil || p.levels == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	if level <= 0 {
		deleter, ok := p.levels.(skillLevelDeleter)
		if !ok {
			return nil
		}
		if err := deleter.DeleteKnownSkill(ctx, c.ID, classIndex, skillID); err != nil {
			return fmt.Errorf("delete known skill for character %d: %w", c.ID, err)
		}
		return nil
	}
	writer, ok := p.levels.(skillLevelWriter)
	if !ok {
		return nil
	}
	if err := writer.SetKnownSkill(ctx, c.ID, classIndex, skillID, level); err != nil {
		return fmt.Errorf("set known skill for character %d: %w", c.ID, err)
	}
	return nil
}

// EquipItemStats attaches the stat functions inst's template contributes
// while equipped — item.Template.AttachedSkills passives and
// item.Template.Modifiers equip bonuses — to c's live stat calculators.
// Call once per instance, right after it becomes equipped.
func (p *Persistence) EquipItemStats(c *player.Character, inst *item.Instance, tmpl *item.Template) error {
	if p == nil || c == nil || inst == nil || tmpl == nil {
		return nil
	}
	owner := effect.ItemOwner{Inst: inst, Tmpl: tmpl}
	modFns, err := effect.ItemModifierFuncs(owner)
	if err != nil {
		return fmt.Errorf("apply equip modifiers for character %d item %d: %w", c.ID, inst.ObjectID, err)
	}
	var passiveFns []basefunc.Func
	if tmpl.Weapon == nil || c.WeaponSkillsAllowed(tmpl.Crystal) {
		passiveFns, err = effect.ItemPassiveFuncs(p.skills, owner)
		if err != nil {
			return fmt.Errorf("apply equip passives for character %d item %d: %w", c.ID, inst.ObjectID, err)
		}
	}
	c.AddStatFuncs(modFns)
	c.AddStatFuncs(passiveFns)
	return nil
}

// UnequipItemStats removes every stat function inst previously contributed
// via EquipItemStats. tmpl must be the same template instance EquipItemStats
// was called with, so the owner identity used to attach the functions
// matches the one used to remove them.
func (p *Persistence) UnequipItemStats(c *player.Character, inst *item.Instance, tmpl *item.Template) {
	if c == nil || inst == nil {
		return
	}
	c.RemoveStatsByOwner(effect.ItemOwner{Inst: inst, Tmpl: tmpl})
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
		if err := p.EquipItemStats(c, inst, tmpl); err != nil {
			return err
		}
	}
	return nil
}

// RefreshEquippedItemStats reapplies equipped item modifiers and passives
// after a state gate changes which passives may be active.
func (p *Persistence) RefreshEquippedItemStats(c *player.Character, inv *itemcontainer.Inventory) error {
	if c == nil || inv == nil {
		return nil
	}
	for _, inst := range inv.PaperdollItems() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			continue
		}
		p.UnequipItemStats(c, inst, tmpl)
		if err := p.EquipItemStats(c, inst, tmpl); err != nil {
			return err
		}
	}
	return nil
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
	return true, len(def.Effects) > 0 || len(def.SelfEffects) > 0
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
