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

func (areaHandler) CanCast(caster, target Actor, skill *modelskill.Definition, ctrl bool) bool {
	return areaCastRejection(caster, target, skill, ctrl) == CastRejectNone
}

// areaCastRejection gates an offensive area skill on its aimed target: a
// playable on the caster's own side or one the caster itself may not hit
// offensively (the social policy reads the caster's own zone and cast
// intention, so a summon answers for itself), or a target it may not attack
// (without a forced attack by its acting player, unless CTRL is held), is an
// invalid target.
func areaCastRejection(caster, target Actor, skill *modelskill.Definition, ctrl bool) CastRejection {
	if skill == nil || !skill.Offensive {
		return CastRejectNone
	}
	if !aimedAreaTarget(caster, target) {
		return CastRejectSilent
	}
	if isPlayable(target) && (ownSide(caster, target) || !caster.CanCastOnPlayable(target, skill, ctrl, true)) {
		return CastRejectInvalidTarget
	}
	return aimedAttackRejection(caster, target, ctrl)
}

// frontAreaCastRejection is areaCastRejection without the playable policy
// check.
func frontAreaCastRejection(caster, target Actor, skill *modelskill.Definition, ctrl bool) CastRejection {
	if skill == nil || !skill.Offensive {
		return CastRejectNone
	}
	if !aimedAreaTarget(caster, target) {
		return CastRejectSilent
	}
	return aimedAttackRejection(caster, target, ctrl)
}

// aimedAreaTarget reports a target an aimed area skill can center on: a
// living creature other than the caster. Anything else has no final target
// and is dropped before any condition message.
func aimedAreaTarget(caster, target Actor) bool {
	return target != nil && !sameCreature(caster, target) && !target.Dead()
}

// aimedAttackRejection refuses a target the caster may not attack, or may
// not attack without a forced attack unless CTRL is held. The forced-attack
// relation is judged against the caster's acting player, so a summon's cast
// follows its owner's.
func aimedAttackRejection(caster, target Actor, ctrl bool) CastRejection {
	if !target.AttackableBy(caster) {
		return CastRejectInvalidTarget
	}
	if !ctrl {
		if player, _ := actingPlayerOf(caster); !target.AttackableWithoutForceBy(player) {
			return CastRejectInvalidTarget
		}
	}
	return CastRejectNone
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
	areaHandler(h).forEachAreaTarget(caster, target, skillRadius(skill), func(creature Actor) bool {
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

func (frontAreaHandler) CanCast(caster, target Actor, skill *modelskill.Definition, ctrl bool) bool {
	return frontAreaCastRejection(caster, target, skill, ctrl) == CastRejectNone
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
	return skill == nil || !skill.Offensive || !playableInPeaceZone(caster)
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
	return auraHandler(h).collect(caster, skillRadius(skill), func(creature Actor) bool {
		return creatureOrientedLocation(caster).IsInFrontOf(creatureLocation(creature))
	}, areaCanAffect)
}

func (frontAuraHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	return caster
}

func (frontAuraHandler) CanCast(caster, _ Actor, skill *modelskill.Definition, _ bool) bool {
	return skill == nil || !skill.Offensive || !playableInPeaceZone(caster)
}

type behindAuraHandler struct {
	known Known
}

func (behindAuraHandler) Target() modelskill.Target { return modelskill.TargetBehindAura }

func (h behindAuraHandler) Targets(caster, _ Actor, skill *modelskill.Definition) []Actor {
	return auraHandler(h).collect(caster, skillRadius(skill), func(creature Actor) bool {
		return creatureOrientedLocation(caster).IsBehind(creatureLocation(creature))
	}, areaCanAffect)
}

func (behindAuraHandler) FinalTarget(caster, _ Actor, _ *modelskill.Definition) Actor {
	return caster
}

func (behindAuraHandler) CanCast(caster, _ Actor, _ *modelskill.Definition, _ bool) bool {
	return !playableInPeaceZone(caster)
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
	return skill == nil || !skill.Offensive || !playableInPeaceZone(caster)
}
