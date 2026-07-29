package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type GroundTargeter interface {
	GroundTarget() (x, y, z int)
	CanSeePoint(x, y, z int) bool
	EffectRangeInPeaceZone(x, y, z, effectRange int) bool
}

type groundHandler struct{}

func (groundHandler) Target() modelskill.Target { return modelskill.TargetGround }

func (groundHandler) Targets(caster, _ Creature, _ *modelskill.Definition) []Creature {
	return []Creature{caster}
}

func (groundHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

// CanCast gates a ground-targeted cast on the caster's last ground-click
// point: real line of sight from the caster to that point, then whether the
// skill's effect range around that point overlaps a peace zone attached to
// the caster's own region. A caster that doesn't track a ground-click point
// (e.g. a test double, or a non-player caster) is permissive, matching the
// reference restricting this target type to players.
func (groundHandler) CanCast(caster, _ Creature, skill *modelskill.Definition, _ bool) bool {
	gt, ok := caster.(GroundTargeter)
	if !ok {
		return true
	}
	x, y, z := gt.GroundTarget()
	if !gt.CanSeePoint(x, y, z) {
		return false
	}
	var effectRange int
	if skill != nil {
		effectRange = skill.EffectRange
	}
	return !gt.EffectRangeInPeaceZone(x, y, z, effectRange)
}
