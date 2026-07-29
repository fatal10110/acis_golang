package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type areaHandler struct {
	known Known
}

func (areaHandler) Target() modelskill.Target { return modelskill.TargetArea }

func (h areaHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if target == nil {
		return nil
	}
	out := []Creature{target}
	h.forEachAreaTarget(caster, target, skillRadius(skill), nil, func(creature Creature) {
		out = append(out, creature)
	})
	return out
}

func (areaHandler) FinalTarget(caster, target Creature, _ *modelskill.Definition) Creature {
	if target == nil || sameCreature(caster, target) || target.Dead() {
		return nil
	}
	return target
}

func (h areaHandler) CanCast(caster, target Creature, skill *modelskill.Definition, ctrl bool) bool {
	if skill == nil || !skill.Offensive {
		return true
	}
	if h.FinalTarget(caster, target, skill) == nil {
		return false
	}
	if !attackableBy(target, caster) {
		return false
	}
	return ctrl || attackableWithoutForceBy(target, caster)
}

func (h areaHandler) forEachAreaTarget(caster, anchor Creature, radius int, keep func(Creature) bool, fn func(Creature)) {
	if h.known == nil {
		return
	}
	h.known.ForEachKnownCreatureInRadius(anchor, radius, func(creature Creature) {
		if sameCreature(caster, creature) || creature.Dead() || !canSee(anchor, creature) {
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

func (h frontAreaHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if target == nil {
		return nil
	}
	out := []Creature{target}
	areaHandler{known: h.known}.forEachAreaTarget(caster, target, skillRadius(skill), func(creature Creature) bool {
		return creatureOrientedLocation(caster).IsInFrontOf(creatureLocation(creature))
	}, func(creature Creature) {
		out = append(out, creature)
	})
	return out
}

func (frontAreaHandler) FinalTarget(caster, target Creature, _ *modelskill.Definition) Creature {
	if target == nil || sameCreature(caster, target) || target.Dead() {
		return nil
	}
	return target
}

func (h frontAreaHandler) CanCast(caster, target Creature, skill *modelskill.Definition, ctrl bool) bool {
	return areaHandler{known: h.known}.CanCast(caster, target, skill, ctrl)
}

type auraHandler struct {
	known Known
}

func (auraHandler) Target() modelskill.Target { return modelskill.TargetAura }

func (h auraHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	return h.collect(caster, skillRadius(skill), nil)
}

func (auraHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (auraHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool { return true }

func (h auraHandler) collect(caster Creature, radius int, keep func(Creature) bool) []Creature {
	if h.known == nil {
		return nil
	}
	var out []Creature
	h.known.ForEachKnownCreatureInRadius(caster, radius, func(creature Creature) {
		if creature.Dead() || !canSee(caster, creature) {
			return
		}
		if keep != nil && !keep(creature) {
			return
		}
		if auraCanAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

type frontAuraHandler struct {
	known Known
}

func (frontAuraHandler) Target() modelskill.Target { return modelskill.TargetFrontAura }

func (h frontAuraHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	return auraHandler{known: h.known}.collect(caster, skillRadius(skill), func(creature Creature) bool {
		return creatureOrientedLocation(caster).IsInFrontOf(creatureLocation(creature))
	})
}

func (frontAuraHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (frontAuraHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool {
	return true
}

type behindAuraHandler struct {
	known Known
}

func (behindAuraHandler) Target() modelskill.Target { return modelskill.TargetBehindAura }

func (h behindAuraHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	return auraHandler{known: h.known}.collect(caster, skillRadius(skill), func(creature Creature) bool {
		return creatureOrientedLocation(caster).IsBehind(creatureLocation(creature))
	})
}

func (behindAuraHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (behindAuraHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool {
	return true
}

type auraUndeadHandler struct {
	known Known
}

func (auraUndeadHandler) Target() modelskill.Target { return modelskill.TargetAuraUndead }

func (h auraUndeadHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	if h.known == nil {
		return nil
	}
	var out []Creature
	h.known.ForEachKnownCreatureInRadius(caster, skillRadius(skill), func(creature Creature) {
		if creature.Dead() || !isUndead(creature) || !canSee(caster, creature) {
			return
		}
		if areaCanAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

func (auraUndeadHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (auraUndeadHandler) CanCast(caster, _ Creature, skill *modelskill.Definition, _ bool) bool {
	return skill == nil || !skill.Offensive || !inPeaceZone(caster)
}
