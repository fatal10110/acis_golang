package target

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// GroundCastFailure identifies why a ground-targeted cast cannot start.
type GroundCastFailure uint8

const (
	GroundCastAllowed GroundCastFailure = iota
	GroundCastNoLineOfSight
	GroundCastPeaceZone
)

type groundHandler struct{}

func (groundHandler) Target() modelskill.Target { return modelskill.TargetGround }

func (groundHandler) Targets(caster, _ Actor, _ *modelskill.Definition) []Actor {
	return []Actor{caster}
}

func (groundHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	return caster
}

// GroundCastFailureFor checks the caster's last ground-click
// point: real line of sight from the caster to that point, then whether the
// skill's effect range around that point overlaps a peace zone attached to
// the caster's own region. Only players track a ground-click point, so any
// other caster is permissive, matching the reference restricting this target
// type to players.
func GroundCastFailureFor(caster Actor, skill *modelskill.Definition) GroundCastFailure {
	if caster.Kind() != actor.KindPlayer {
		return GroundCastAllowed
	}
	x, y, z := caster.GroundTarget()
	if !caster.CanSeePoint(x, y, z) {
		return GroundCastNoLineOfSight
	}
	var effectRange int
	if skill != nil {
		effectRange = skill.EffectRange
	}
	if caster.EffectRangeInPeaceZone(x, y, z, effectRange) {
		return GroundCastPeaceZone
	}
	return GroundCastAllowed
}

func (groundHandler) CanCast(caster, _ Actor, skill *modelskill.Definition, _ bool) bool {
	return GroundCastFailureFor(caster, skill) == GroundCastAllowed
}
