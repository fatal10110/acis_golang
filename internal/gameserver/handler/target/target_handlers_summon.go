package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type summonHandler struct{}

func (summonHandler) Target() modelskill.Target { return modelskill.TargetSummon }

func (summonHandler) Targets(caster, _ Creature, _ *modelskill.Definition) []Creature {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return []Creature{summon}
}

func (summonHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return summon
}

func (summonHandler) CanCast(caster, _ Creature, _ *modelskill.Definition, _ bool) bool {
	summon, ok := summonOf(caster)
	return ok && !summon.Dead()
}

type areaSummonHandler struct {
	known Known
}

func (areaSummonHandler) Target() modelskill.Target { return modelskill.TargetAreaSummon }

func (h areaSummonHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if !caster.Category().Has(CategoryPlayable) || target == nil {
		return nil
	}
	var out []Creature
	areaHandler{known: h.known}.forEachAreaTarget(caster, target, skillRadius(skill), nil, func(creature Creature) {
		out = append(out, creature)
	})
	return out
}

func (areaSummonHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return summon
}

func (areaSummonHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool {
	return true
}

type ownerPetHandler struct{}

func (ownerPetHandler) Target() modelskill.Target { return modelskill.TargetOwnerPet }

func (ownerPetHandler) Targets(caster, _ Creature, _ *modelskill.Definition) []Creature {
	owner, ok := ownerOf(caster)
	if !ok {
		return nil
	}
	return []Creature{owner}
}

func (ownerPetHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	owner, ok := ownerOf(caster)
	if !ok {
		return nil
	}
	return owner
}

func (ownerPetHandler) CanCast(caster, target Creature, _ *modelskill.Definition, _ bool) bool {
	owner, ok := ownerOf(caster)
	return ok && sameCreature(owner, target) && !target.Dead()
}
