package target

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CastRejection identifies a target-handler failure that the cast boundary
// must show to the player. It deliberately does not name packets.
type CastRejection uint8

const (
	CastRejectNone CastRejection = iota
	CastRejectInvalidTarget
	// CastRejectSilent preserves a rejected target without a system message.
	CastRejectSilent
	CastRejectCantAttackPeaceZone
	CastRejectTargetInPeaceZone
	CastRejectCannotUseSkill
	// Corpse-mob failures: harvest on a non-monster corpse, a corpse past
	// half its decay time that is neither seeded nor spoiled, and sweep on a
	// non-monster corpse.
	CastRejectHarvestNotMonster
	CastRejectCorpseTooOld
	CastRejectSweepNotMonster
	// CastRejectCannotUseOnYourself refuses a skill that must not target
	// its caster.
	CastRejectCannotUseOnYourself
	// CastRejectOlympiadUnavailable refuses a skill barred during the
	// Olympiad.
	CastRejectOlympiadUnavailable
)

// CastRejectionFor classifies a playable caster's target conditions: every
// failure the handler's CanCast reports, with the system message the player
// is shown, or CastRejectSilent when there is none. GROUND is checked
// separately (GroundCastFailureFor). A nil target always classifies as
// CastRejectNone: a missing target is dropped before the cast stops the
// caster, not rejected after it.
func CastRejectionFor(targetType modelskill.Target, caster, target Actor, skill *modelskill.Definition, ctrl bool) CastRejection {
	switch targetType {
	case modelskill.TargetAura, modelskill.TargetFrontAura, modelskill.TargetAuraUndead:
		if skill != nil && skill.Offensive && caster.InPeaceZone() {
			return CastRejectCantAttackPeaceZone
		}
	case modelskill.TargetBehindAura:
		if caster.InPeaceZone() {
			return CastRejectCantAttackPeaceZone
		}
	case modelskill.TargetOne:
		return oneCastRejection(caster, target, skill, ctrl)
	case modelskill.TargetArea:
		if target == nil {
			return CastRejectNone
		}
		return areaCastRejection(caster, target, skill, ctrl)
	case modelskill.TargetFrontArea:
		if target == nil {
			return CastRejectNone
		}
		return frontAreaCastRejection(caster, target, skill, ctrl)
	case modelskill.TargetOwnerPet:
		if target == nil {
			return CastRejectNone
		}
		return ownerPetCastRejection(caster, target)
	case modelskill.TargetPartyMember:
		if target == nil {
			return CastRejectNone
		}
		return partyMemberCastRejection(caster, target, skill)
	case modelskill.TargetPartyOther:
		if target == nil {
			return CastRejectNone
		}
		return partyOtherCastRejection(caster, target, skill)
	case modelskill.TargetCorpseAlly:
		return corpseAllyCastRejection(caster)
	case modelskill.TargetSummon:
		// The final target is the caster's summon; a dead one is invalid.
		if target != nil && target.Dead() {
			return CastRejectInvalidTarget
		}
	case modelskill.TargetHoly:
		if target == nil {
			return CastRejectNone
		}
		return holyCastRejection(target)
	case modelskill.TargetUnlockable:
		if target == nil {
			return CastRejectNone
		}
		return unlockableCastRejection(target)
	case modelskill.TargetUndead:
		if target == nil {
			return CastRejectNone
		}
		return undeadCastRejection(target)
	case modelskill.TargetCorpseMob, modelskill.TargetAreaCorpseMob:
		if target == nil {
			return CastRejectNone
		}
		return corpseMobCastRejection(target, skill)
	case modelskill.TargetCorpsePlayer:
		return corpsePlayerCastRejection(target)
	case modelskill.TargetCorpsePet:
		return corpsePetCastRejection(target)
	}
	return CastRejectNone
}

func holyCastRejection(target Actor) CastRejection {
	if !target.Holy() {
		return CastRejectInvalidTarget
	}
	return CastRejectNone
}

// undeadCastRejection splits the UNDEAD target check: a target that is not
// a living monster or servitor is an invalid target, while a living one that
// is not undead refuses the skill by name.
func undeadCastRejection(target Actor) CastRejection {
	if target.Dead() || !undeadTargetKind(target) {
		return CastRejectInvalidTarget
	}
	if !target.Undead() {
		return CastRejectCannotUseSkill
	}
	return CastRejectNone
}

func unlockableCastRejection(target Actor) CastRejection {
	if target.Unlockable() {
		return CastRejectNone
	}
	if target.Kind() == actor.KindDoor {
		return CastRejectSilent
	}
	return CastRejectInvalidTarget
}

func oneCastRejection(caster, target Actor, skill *modelskill.Definition, ctrl bool) CastRejection {
	if target == nil || skill == nil {
		return CastRejectNone
	}
	if !skill.Offensive {
		if isPlayable(target) {
			if !caster.CanCastOnPlayable(target, skill, ctrl, false) {
				return CastRejectInvalidTarget
			}
			return CastRejectNone
		}
		if target.MonsterKind() && !ctrl && !skill.IsDamage() && !skill.Debuff {
			return CastRejectInvalidTarget
		}
		return CastRejectNone
	}
	if sameCreature(caster, target) || target.Dead() {
		return CastRejectInvalidTarget
	}
	if isPlayable(target) {
		if !caster.CanCastOnPlayable(target, skill, ctrl, true) {
			return CastRejectInvalidTarget
		}
		if !target.AttackableBy(caster) || (!ctrl && !target.AttackableWithoutForceBy(caster)) {
			return CastRejectInvalidTarget
		}
		if caster.OlympiadMode() && !caster.OlympiadStarted() {
			return CastRejectInvalidTarget
		}
		if caster.InPeaceZone() {
			return CastRejectCantAttackPeaceZone
		}
		if target.InPeaceZone() {
			return CastRejectTargetInPeaceZone
		}
		return CastRejectNone
	}
	if target.FolkOrGuard() {
		if !ctrl || !skill.IsDamage() {
			return CastRejectInvalidTarget
		}
		return CastRejectNone
	}
	if target.Kind() == actor.KindDoor {
		if !target.AttackableBy(caster) {
			return CastRejectInvalidTarget
		}
	}
	return CastRejectNone
}
