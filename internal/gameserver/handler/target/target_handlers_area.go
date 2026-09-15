package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type areaHandler struct {
	known Known
}

func (areaHandler) Target() modelskill.Target { return modelskill.TargetArea }

func (h areaHandler) Targets(caster, target Actor, skill *modelskill.Definition) []Actor {
	if target == nil {
		return nil
	}
	out := []Actor{target}
	h.forEachAreaTarget(caster, target, skillRadius(skill), nil, func(creature Actor) {
		out = append(out, creature)
	})
	return out
}

func (areaHandler) FinalTarget(caster, target Actor, _ *modelskill.Definition) Actor {
	if target == nil || sameCreature(caster, target) || target.Dead() {
		return nil
	}
	return target
}

func (h areaHandler) CanCast(caster, target Actor, skill *modelskill.Definition, ctrl bool) bool {
	if skill == nil || !skill.Offensive {
		return true
	}
	if h.FinalTarget(caster, target, skill) == nil {
		return false
	}
	if !target.AttackableBy(caster) {
		return false
	}
	return ctrl || target.AttackableWithoutForceBy(caster)
}

func (h areaHandler) forEachAreaTarget(caster, anchor Actor, radius int, keep func(Actor) bool, fn func(Actor)) {
	if h.known == nil {
		return
	}
	h.known.ForEachKnownCreatureInRadius(anchor, radius, func(creature Actor) {
		if sameCreature(caster, creature) || creature.Dead() || !anchor.CanSeeTarget(creature) {
			return
		}
		if keep != nil && !keep(creature) {
			return
		}
		if areaCanAffect(caster, creature) {
			fn(creature)
		}
	})
}

type frontAreaHandler struct {
	known Known
}

func (frontAreaHandler) Target() modelskill.Target { return modelskill.TargetFrontArea }

func (h frontAreaHandler) Targets(caster, target Actor, skill *modelskill.Definition) []Actor {
	if target == nil {
		return nil
	}
	out := []Actor{target}
	areaHandler{known: h.known}.forEachAreaTarget(caster, target, skillRadius(skill), func(creature Actor) bool {
		return creatureOrientedLocation(caster).IsInFrontOf(creatureLocation(creature))
	}, func(creature Actor) {
		out = append(out, creature)
	})
	return out
}

func (frontAreaHandler) FinalTarget(caster, target Actor, _ *modelskill.Definition) Actor {
	if target == nil || sameCreature(caster, target) || target.Dead() {
		return nil
	}
	return target
}

func (h frontAreaHandler) CanCast(caster, target Actor, skill *modelskill.Definition, ctrl bool) bool {
	return areaHandler{known: h.known}.CanCast(caster, target, skill, ctrl)
}

type auraHandler struct {
	known Known
}

func (auraHandler) Target() modelskill.Target { return modelskill.TargetAura }

func (h auraHandler) Targets(caster, _ Actor, skill *modelskill.Definition) []Actor {
	return h.collect(caster, skillRadius(skill), nil, auraCanAffect)
}

func (auraHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	return caster
}

func (auraHandler) CanCast(caster, _ Actor, skill *modelskill.Definition, _ bool) bool {
	return skill == nil || !skill.Offensive || !caster.InPeaceZone()
}

func (h auraHandler) collect(caster Actor, radius int, keep func(Actor) bool, canAffect func(Actor, Actor) bool) []Actor {
	if h.known == nil {
		return nil
	}
	var out []Actor
	h.known.ForEachKnownCreatureInRadius(caster, radius, func(creature Actor) {
		if creature.Dead() || !caster.CanSeeTarget(creature) {
			return
		}
		if keep != nil && !keep(creature) {
			return
		}
		if canAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

type frontAuraHandler struct {
	known Known
}

func (frontAuraHandler) Target() modelskill.Target { return modelskill.TargetFrontAura }

func (h frontAuraHandler) Targets(caster, _ Actor, skill *modelskill.Definition) []Actor {
	return auraHandler{known: h.known}.collect(caster, skillRadius(skill), func(creature Actor) bool {
		return creatureOrientedLocation(caster).IsInFrontOf(creatureLocation(creature))
	}, areaCanAffect)
}

func (frontAuraHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	return caster
}

func (frontAuraHandler) CanCast(caster, _ Actor, skill *modelskill.Definition, _ bool) bool {
	return skill == nil || !skill.Offensive || !caster.InPeaceZone()
}

type behindAuraHandler struct {
	known Known
}

func (behindAuraHandler) Target() modelskill.Target { return modelskill.TargetBehindAura }

func (h behindAuraHandler) Targets(caster, _ Actor, skill *modelskill.Definition) []Actor {
	return auraHandler{known: h.known}.collect(caster, skillRadius(skill), func(creature Actor) bool {
		return creatureOrientedLocation(caster).IsBehind(creatureLocation(creature))
	}, areaCanAffect)
}

func (behindAuraHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	return caster
}

func (behindAuraHandler) CanCast(caster, _ Actor, _ *modelskill.Definition, _ bool) bool {
	return !caster.InPeaceZone()
}

type auraUndeadHandler struct {
	known Known
}

func (auraUndeadHandler) Target() modelskill.Target { return modelskill.TargetAuraUndead }

func (h auraUndeadHandler) Targets(caster, _ Actor, skill *modelskill.Definition) []Actor {
	if h.known == nil {
		return nil
	}
	var out []Actor
	h.known.ForEachKnownCreatureInRadius(caster, skillRadius(skill), func(creature Actor) {
		if creature.Dead() || !creature.Undead() || !caster.CanSeeTarget(creature) {
			return
		}
		if areaCanAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

func (auraUndeadHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	return caster
}

func (auraUndeadHandler) CanCast(caster, _ Actor, skill *modelskill.Definition, _ bool) bool {
	return skill == nil || !skill.Offensive || !caster.InPeaceZone()
}
