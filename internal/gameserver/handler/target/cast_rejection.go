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
func CastRejectionFor(targetType modelskill.Target, caster, target Creature, skill *modelskill.Definition, ctrl bool) CastRejection {
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
		return oneCastRejection(caster, target, skill, ctrl)
	}
	return CastRejectNone
}

func oneCastRejection(caster, target Creature, skill *modelskill.Definition, ctrl bool) CastRejection {
	if target == nil || skill == nil {
		return CastRejectNone
	}
	if !skill.Offensive {
		if target.Category().Has(CategoryPlayable) {
			if rules, ok := caster.(PlayableCastRules); ok && !rules.CanCastOnPlayable(target, skill, ctrl, false) {
				return CastRejectInvalidTarget
			}
			return CastRejectNone
		}
		if monster, ok := target.(MonsterTarget); ok && monster.MonsterKind() && !ctrl && !skillIsDamage(skill) && !skill.Debuff {
			return CastRejectInvalidTarget
		}
		return CastRejectNone
	}
	if sameCreature(caster, target) || target.Dead() {
		return CastRejectInvalidTarget
	}
	if target.Category().Has(CategoryPlayable) {
		if rules, ok := caster.(PlayableCastRules); ok && !rules.CanCastOnPlayable(target, skill, ctrl, true) {
			return CastRejectInvalidTarget
		}
		if rules, ok := target.(AttackRules); ok && (!rules.AttackableBy(caster) || (!ctrl && !rules.AttackableWithoutForceBy(caster))) {
			return CastRejectInvalidTarget
		}
		if olympiad, ok := caster.(OlympiadCastState); ok && olympiad.OlympiadMode() && !olympiad.OlympiadStarted() {
			return CastRejectInvalidTarget
		}
		if inPeaceZone(caster) {
			return CastRejectCantAttackPeaceZone
		}
		if inPeaceZone(target) {
			return CastRejectTargetInPeaceZone
		}
		return CastRejectNone
	}
	if folk, ok := target.(FolkOrGuardTarget); ok && folk.FolkOrGuard() {
		if !ctrl || !skillIsDamage(skill) {
			return CastRejectInvalidTarget
		}
		return CastRejectNone
	}
	if door, ok := target.(DoorTarget); ok && door.Door() {
		if rules, ok := target.(AttackRules); !ok || !rules.AttackableBy(caster) {
			return CastRejectInvalidTarget
		}
	}
	return CastRejectNone
}

func skillIsDamage(skill *modelskill.Definition) bool {
	switch skill.SkillType {
	case "PDAM", "MDAM", "DRAIN", "BLOW", "CPDAMPERCENT", "DEATHLINK", "CHARGEDAM", "FATAL", "SIGNET_CASTTIME":
		return true
	}
	return false
}
