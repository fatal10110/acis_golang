package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type corpseMobHandler struct{}

func (corpseMobHandler) Target() modelskill.Target { return modelskill.TargetCorpseMob }

func (corpseMobHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (corpseMobHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (corpseMobHandler) CanCast(_, target Creature, skill *modelskill.Definition, _ bool) bool {
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

func (h areaCorpseMobHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if target == nil {
		return nil
	}
	out := []Creature{target}
	if h.known == nil {
		return out
	}
	h.known.ForEachKnownCreatureInRadius(target, skillRadius(skill), func(creature Creature) {
		if sameCreature(caster, creature) || !canSee(target, creature) {
			return
		}
		if skill != nil && skill.ID == harvestGrandBoxSkillID {
			if creature.Category().Has(CategoryAttackable) && creature.Dead() {
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

func (areaCorpseMobHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (areaCorpseMobHandler) CanCast(_, target Creature, skill *modelskill.Definition, _ bool) bool {
	return corpseMobCanCast(target, skill)
}

// CorpseCastFailure identifies why a corpse-mob cast cannot start.
type CorpseCastFailure uint8

const (
	CorpseCastAllowed CorpseCastFailure = iota
	CorpseCastInvalidTarget
	CorpseCastHarvestNotMonster
	CorpseCastTooOld
	CorpseCastSweepNotMonster
)

func corpseMobCanCast(target Creature, skill *modelskill.Definition) bool {
	return CorpseCastFailureFor(target, skill) == CorpseCastAllowed
}

// CorpseCastFailureFor applies the shared corpse-mob eligibility rule.
func CorpseCastFailureFor(target Creature, skill *modelskill.Definition) CorpseCastFailure {
	if target == nil || !hasCorpse(target) || target.Category().Has(CategoryPlayable) {
		return CorpseCastInvalidTarget
	}
	if skill != nil && skill.SkillType == "HARVEST" {
		if !monsterKind(target) {
			return CorpseCastHarvestNotMonster
		}
		return CorpseCastAllowed
	}
	if target.Category().Has(CategoryAttackable) && corpseTooOld(target) && !corpseAgeBypass(target) {
		return CorpseCastTooOld
	}
	if skill != nil && skill.SkillType == "SWEEP" && !monsterKind(target) {
		return CorpseCastSweepNotMonster
	}
	return CorpseCastAllowed
}

type corpsePlayerHandler struct{}

func (corpsePlayerHandler) Target() modelskill.Target { return modelskill.TargetCorpsePlayer }

func (corpsePlayerHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (corpsePlayerHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (corpsePlayerHandler) CanCast(_, target Creature, _ *modelskill.Definition, _ bool) bool {
	return target != nil && target.Dead() && target.Category().Has(CategoryPlayable)
}

type corpsePetHandler struct{}

func (corpsePetHandler) Target() modelskill.Target { return modelskill.TargetCorpsePet }

func (corpsePetHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (corpsePetHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (corpsePetHandler) CanCast(_, target Creature, _ *modelskill.Definition, _ bool) bool {
	return target != nil && corpsePetCastRejection(target) == CastRejectNone
}

func corpsePetCastRejection(target Creature) CastRejection {
	if target == nil {
		return CastRejectNone
	}
	if !target.Dead() {
		return CastRejectInvalidTarget
	}
	pet, ok := target.(PetTarget)
	if !ok || !pet.IsPet() {
		return CastRejectCannotUseSkill
	}
	return CastRejectNone
}

// GroundTargeter is implemented by casters that track a pending
// ground-click point for ground-targeted skills (signets and similar), and
// can answer the point-based line-of-sight and peace-zone queries
