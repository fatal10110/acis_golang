package script

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// SkillInfo is what a behavior reads of one skill level.
type SkillInfo struct {
	def skill.Definition
}

// Skill returns the definition of ref. A skill the table does not hold
// panics, so the invocation aborts there.
func (s *Script) Skill(ref skill.Ref) SkillInfo {
	def, ok := s.skillDef(ref)
	if !ok {
		panic(fmt.Sprintf("script: no skill %d level %d", ref.ID, ref.Level))
	}
	return SkillInfo{def: def}
}

func (s *Script) skillDef(ref skill.Ref) (skill.Definition, bool) {
	if s.env == nil || s.env.Skills == nil {
		return skill.Definition{}, false
	}
	return s.env.Skills.Definition(ref)
}

// AggroPoints returns the hate the skill adds.
func (k SkillInfo) AggroPoints() int { return k.def.AggroPoints }

// Offensive reports whether the skill is offensive.
func (k SkillInfo) Offensive() bool { return k.def.Offensive }

// Power returns the skill's power as it hits c: a death link or a fatal
// blow grows as c's HP runs low; nil c, and every other skill, gives the
// skill's own power.
func (k SkillInfo) Power(c Creature) float64 {
	power := float64(k.def.Power)
	if c == nil || combatantOf(c) == nil {
		return power
	}
	switch k.def.SkillType {
	case "DEATHLINK":
		return power * math.Pow(1.7165-c.HPRatio(), 2) * 0.577
	case "FATAL":
		return power + power*math.Pow(1.7165-c.HPRatio(), 3.5)*0.577
	}
	return power
}

// effectHolder is a creature that holds effects.
type effectHolder interface {
	EffectList() *effect.List
}

// AbnormalLevel returns the level at which c holds an effect of the kind
// ref's effects stack as, -1 when c holds none or ref is no skill. For each
// effect c holds and each effect of ref, one of the same stack type sets
// the level to the larger of its skill's abnormal level and its stack
// order, truncated; a later match replaces an earlier one.
func (s *Script) AbnormalLevel(c Creature, ref skill.Ref) int32 {
	def, ok := s.skillDef(ref)
	if !ok {
		return -1
	}
	holder, ok := tracked(c).(effectHolder)
	if !ok {
		return -1
	}
	level := int32(-1)
	for _, e := range holder.EffectList().All() {
		for _, t := range def.Effects {
			if e.Template.StackType == t.StackType {
				level = int32(max(e.Skill.AbnormalLevel, int(e.Template.StackOrder)))
			}
		}
	}
	return level
}
