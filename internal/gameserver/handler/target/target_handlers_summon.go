package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type summonHandler struct{}

func (summonHandler) Target() modelskill.Target { return modelskill.TargetSummon }

func (summonHandler) Targets(caster, _ Actor, _ *modelskill.Definition) []Actor {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return []Actor{summon}
}

func (summonHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return summon
}

func (summonHandler) CanCast(caster, _ Actor, _ *modelskill.Definition, _ bool) bool {
	summon, ok := summonOf(caster)
	return ok && !summon.Dead()
}

type areaSummonHandler struct {
	known Known
}

func (areaSummonHandler) Target() modelskill.Target { return modelskill.TargetAreaSummon }

func (h areaSummonHandler) Targets(caster, target Actor, skill *modelskill.Definition) []Actor {
	if !isPlayable(caster) || target == nil {
		return nil
	}
	var out []Actor
	areaHandler{known: h.known}.forEachAreaTarget(caster, target, skillRadius(skill), nil, func(creature Actor) {
		out = append(out, creature)
	})
	return out
}

func (areaSummonHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return summon
}

func (areaSummonHandler) CanCast(Actor, Actor, *modelskill.Definition, bool) bool {
	return true
}

type ownerPetHandler struct{}

func (ownerPetHandler) Target() modelskill.Target { return modelskill.TargetOwnerPet }

func (ownerPetHandler) Targets(caster, _ Actor, _ *modelskill.Definition) []Actor {
	owner, ok := ownerOf(caster)
	if !ok {
		return nil
	}
	return []Actor{owner}
}

func (ownerPetHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	owner, ok := ownerOf(caster)
	if !ok {
		return nil
	}
	return owner
}

func (ownerPetHandler) CanCast(caster, target Actor, _ *modelskill.Definition, _ bool) bool {
	owner, ok := ownerOf(caster)
	return ok && sameCreature(owner, target) && !target.Dead()
}
