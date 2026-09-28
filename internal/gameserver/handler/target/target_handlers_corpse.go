package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type corpseMobHandler struct{}

func (corpseMobHandler) Target() modelskill.Target { return modelskill.TargetCorpseMob }

func (corpseMobHandler) Targets(_, target Actor, _ *modelskill.Definition) []Actor {
	return []Actor{target}
}

func (corpseMobHandler) FinalTarget(_, target Actor, _ *modelskill.Definition) Actor {
	return target
}

func (corpseMobHandler) CanCast(_, target Actor, skill *modelskill.Definition, _ bool) bool {
	return corpseMobCanCast(target, skill)
}

type areaCorpseMobHandler struct {
	known Known
}

func (areaCorpseMobHandler) Target() modelskill.Target { return modelskill.TargetAreaCorpseMob }

// harvestGrandBoxSkillID is the one skill (Harvest Grand Box, id 444) that
// widens the corpse-mob area scan to also sweep in every already-dead
// attackable creature nearby, instead of the usual live-target splash.
const harvestGrandBoxSkillID = 444

func (h areaCorpseMobHandler) Targets(caster, target Actor, skill *modelskill.Definition) []Actor {
	if target == nil {
		return nil
	}
	out := []Actor{target}
	if h.known == nil {
		return out
	}
	h.known.ForEachKnownCreatureInRadius(target, skillRadius(skill), func(creature Actor) {
		if sameCreature(caster, creature) || !target.CanSeeTarget(creature) {
			return
		}
		if skill != nil && skill.ID == harvestGrandBoxSkillID {
			if isAttackable(creature) && creature.Dead() {
				out = append(out, creature)
			}
			return
		}
		if creature.Dead() {
			return
		}
		if areaCanAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

func (areaCorpseMobHandler) FinalTarget(_, target Actor, _ *modelskill.Definition) Actor {
	return target
}

func (areaCorpseMobHandler) CanCast(_, target Actor, skill *modelskill.Definition, _ bool) bool {
	return corpseMobCanCast(target, skill)
}

func corpseMobCanCast(target Actor, skill *modelskill.Definition) bool {
	return target != nil && corpseMobCastRejection(target, skill) == CastRejectNone
}

// corpseMobCastRejection applies the shared corpse-mob eligibility rule.
func corpseMobCastRejection(target Actor, skill *modelskill.Definition) CastRejection {
	if !target.HasCorpse() || isPlayable(target) {
		return CastRejectInvalidTarget
	}
	if skill != nil && skill.SkillType == "HARVEST" {
		if !target.MonsterKind() {
			return CastRejectHarvestNotMonster
		}
		return CastRejectNone
	}
	if isAttackable(target) && corpseTooOld(target) && !corpseAgeBypass(target) {
		return CastRejectCorpseTooOld
	}
	if skill != nil && skill.SkillType == "SWEEP" && !target.MonsterKind() {
		return CastRejectSweepNotMonster
	}
	return CastRejectNone
}

type corpsePlayerHandler struct{}

func (corpsePlayerHandler) Target() modelskill.Target { return modelskill.TargetCorpsePlayer }

func (corpsePlayerHandler) Targets(_, target Actor, _ *modelskill.Definition) []Actor {
	return []Actor{target}
}

func (corpsePlayerHandler) FinalTarget(_, target Actor, _ *modelskill.Definition) Actor {
	return target
}

func (corpsePlayerHandler) CanCast(_, target Actor, _ *modelskill.Definition, _ bool) bool {
	return target != nil && corpsePlayerCastRejection(target) == CastRejectNone
}

func corpsePlayerCastRejection(target Actor) CastRejection {
	if target == nil {
		return CastRejectNone
	}
	if !target.Dead() {
		return CastRejectInvalidTarget
	}
	if !isPlayable(target) {
		return CastRejectCannotUseSkill
	}
	return CastRejectNone
}

type corpsePetHandler struct{}

func (corpsePetHandler) Target() modelskill.Target { return modelskill.TargetCorpsePet }

func (corpsePetHandler) Targets(_, target Actor, _ *modelskill.Definition) []Actor {
	return []Actor{target}
}

func (corpsePetHandler) FinalTarget(_, target Actor, _ *modelskill.Definition) Actor {
	return target
}

func (corpsePetHandler) CanCast(_, target Actor, _ *modelskill.Definition, _ bool) bool {
	return target != nil && corpsePetCastRejection(target) == CastRejectNone
}

func corpsePetCastRejection(target Actor) CastRejection {
	if target == nil {
		return CastRejectNone
	}
	if !target.Dead() {
		return CastRejectInvalidTarget
	}
	if !target.IsPet() {
		return CastRejectCannotUseSkill
	}
	return CastRejectNone
}

// GroundTargeter is implemented by casters that track a pending
// ground-click point for ground-targeted skills (signets and similar), and
// can answer the point-based line-of-sight and peace-zone queries
