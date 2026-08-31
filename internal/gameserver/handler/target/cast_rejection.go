package target

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

// CastRejection identifies a target-handler failure that the cast boundary
// must show to the player. It deliberately does not name packets.
type CastRejection uint8

const (
	CastRejectNone CastRejection = iota
	CastRejectInvalidTarget
	CastRejectCantAttackPeaceZone
	CastRejectTargetInPeaceZone
)

// CastRejectionFor classifies the target-handler failures for which the
// reference sends a system message. Other failed target checks remain silent.
func CastRejectionFor(targetType modelskill.Target, caster, target Creature, skill *modelskill.Definition) CastRejection {
	switch targetType {
	case modelskill.TargetAura, modelskill.TargetFrontAura:
		if skill != nil && skill.Offensive && inPeaceZone(caster) {
			return CastRejectCantAttackPeaceZone
		}
	case modelskill.TargetBehindAura:
		if inPeaceZone(caster) {
			return CastRejectCantAttackPeaceZone
		}
	case modelskill.TargetOne:
		return oneCastRejection(caster, target, skill)
	}
	return CastRejectNone
}

func oneCastRejection(caster, target Creature, skill *modelskill.Definition) CastRejection {
	if target == nil || skill == nil || !skill.Offensive {
		return CastRejectNone
	}
	if sameCreature(caster, target) || target.Dead() {
		return CastRejectInvalidTarget
	}
	if inPeaceZone(caster) {
		return CastRejectCantAttackPeaceZone
	}
	if target.Category().Has(CategoryPlayable) && inPeaceZone(target) {
		return CastRejectTargetInPeaceZone
	}
	return CastRejectNone
}
