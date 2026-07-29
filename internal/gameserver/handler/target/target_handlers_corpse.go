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

// corpseMobCanCast applies the mob-corpse cast-eligibility rule shared by
// the single-target and area corpse-mob handlers: the target must have a
// pending corpse and not be a player-controlled actor or summon, a harvest
// skill always succeeds against an attackable corpse regardless of its age,
// and a sweep skill only ever succeeds against an attackable corpse. Mob
// corpses that expose decay deadline state stop accepting generic corpse
// skills once they are past the halfway age cutoff, unless seeded/spoiled.
func corpseMobCanCast(target Creature, skill *modelskill.Definition) bool {
	if target == nil || !hasCorpse(target) || target.Category().Has(CategoryPlayable) {
		return false
	}
	if skill != nil && skill.SkillType == "HARVEST" {
		return target.Category().Has(CategoryAttackable)
	}
	if skill != nil && skill.SkillType == "SWEEP" && !target.Category().Has(CategoryAttackable) {
		return false
	}
	if target.Category().Has(CategoryAttackable) && corpseTooOld(target) && !corpseAgeBypass(target) {
		return false
	}
	return true
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
	if target == nil || !target.Dead() {
		return false
	}
	pet, ok := target.(PetTarget)
	return ok && pet.IsPet()
}

// GroundTargeter is implemented by casters that track a pending
// ground-click point for ground-targeted skills (signets and similar), and
// can answer the point-based line-of-sight and peace-zone queries
