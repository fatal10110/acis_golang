package script

import (
	"fmt"
	"math"

	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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

// Power returns the skill's power as it hits c, a door included: a death
// link or a fatal blow grows as c's HP runs low; nil c, and every other
// skill, gives the skill's own power.
func (k SkillInfo) Power(c Creature) float64 {
	power := float64(k.def.Power)
	if identity(c) == 0 {
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

// EffectID returns the id of the skill's added effect, 0 for none.
func (k SkillInfo) EffectID() int32 { return int32(k.def.EffectID) }

// Element returns the skill's element.
func (k SkillInfo) Element() skill.Element { return k.def.Element }

// Magic reports whether the skill is magic.
func (k SkillInfo) Magic() bool { return k.def.Magic }

// effectParticipant is a creature that gives and takes effects.
type effectParticipant interface {
	effect.Actor
	skillhandler.Actor
}

// effectActor is c as an effect participant: a player's character, an
// NPC or a summon as the world tracks it. A handle on nothing panics.
func effectActor(c Creature) effectParticipant {
	t := tracked(c)
	if h, ok := t.(player.CharacterHolder); ok {
		return h.PlayerCharacter()
	}
	a, ok := t.(effectParticipant)
	if !ok {
		panic(fmt.Sprintf("script: effects of %T", t))
	}
	return a
}

// ApplyEffects lands the effects of ref on effected from effector at once,
// outside any cast: no cast time, cost, shot or shield. The landing gates
// and rolls of a cast's effects apply: a dead effected, one out of the
// skill's effect range, or one an offensive skill cannot touch takes
// nothing. A player effector is told of each effect with an icon effected
// resisted. A skill the table does not hold panics.
func (s *Script) ApplyEffects(ref skill.Ref, effector, effected Creature) {
	def := s.Skill(ref).def
	from, to := effectActor(effector), effectActor(effected)
	resisted := skillhandler.LandEffects(from, to, def)
	if resisted == 0 {
		return
	}
	if c, ok := from.(*player.Character); ok {
		name := effected.Name()
		for range resisted {
			c.NotifyResistedSkill(name, def.ID, def.Level)
		}
	}
}
