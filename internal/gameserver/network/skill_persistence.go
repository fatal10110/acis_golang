package network

import (
	"context"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

// SkillPersistence saves and restores a live player's buff and skill-reuse
// state at the game-client lifecycle boundary.
type SkillPersistence struct {
	store  skillSaveStore
	levels skillLevelStore
	skills *modelskill.Table
	now    func() time.Time
}

// NewSkillPersistence returns a lifecycle persistence component backed by
// store and the loaded skill table.
func NewSkillPersistence(store skillSaveStore, skills *modelskill.Table, levels ...skillLevelStore) *SkillPersistence {
	p := &SkillPersistence{store: store, skills: skills, now: time.Now}
	if len(levels) > 0 {
		p.levels = levels[0]
	}
	return p
}

// Save replaces c's persisted skill state with its current active effects
// and pending reuse timers.
func (p *SkillPersistence) Save(ctx context.Context, c *player.Character, includeEffects bool) error {
	if p == nil || p.store == nil || c == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	rows := effect.BuildSaveRows(c.ActiveSkillEffects(), c.SkillReuseTimers(p.now()), classIndex, includeEffects)
	if err := p.store.Replace(ctx, c.ID, classIndex, rows); err != nil {
		return fmt.Errorf("save skill state for character %d: %w", c.ID, err)
	}
	return nil
}

// Restore consumes c's persisted skill state, reinstating pending reuse
// timers and effect rows whose skill definitions still exist.
func (p *SkillPersistence) Restore(ctx context.Context, c *player.Character) error {
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
	plan := effect.BuildRestorePlan(rows, p.now().UnixMilli(), p.lookup)
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

// SetKnownSkill records one learned skill on the character and, when the
// backing store can write character_skills, persists it first.
func (p *SkillPersistence) SetKnownSkill(ctx context.Context, c *player.Character, skillID, level int) error {
	if c == nil {
		return nil
	}
	classIndex := c.SkillSaveClassIndex()
	if p != nil && p.levels != nil {
		if writer, ok := p.levels.(skillLevelWriter); ok {
			if err := writer.SetKnownSkill(ctx, c.ID, classIndex, skillID, level); err != nil {
				return fmt.Errorf("set known skill for character %d: %w", c.ID, err)
			}
		}
	}
	c.SetSkillLevel(skillID, level)
	return nil
}

func (p *SkillPersistence) restoreKnownSkills(ctx context.Context, c *player.Character, classIndex int32) error {
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
		if p.skills != nil {
			if _, ok := p.skills.Get(modelskill.ID(id), level); !ok {
				continue
			}
		}
		c.SetSkillLevel(id, level)
	}
	return nil
}

func (p *SkillPersistence) lookup(ref modelskill.Ref) (bool, bool) {
	def, ok := p.definition(ref)
	if !ok {
		return false, false
	}
	return true, len(def.Effects) > 0 || len(def.SelfEffects) > 0
}

func (p *SkillPersistence) definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	if p == nil || p.skills == nil {
		return modelskill.Definition{}, false
	}
	return p.skills.Get(ref.ID, ref.Level)
}

func (p *SkillPersistence) hasDefinition(ref modelskill.Ref) bool {
	_, ok := p.definition(ref)
	return ok
}
