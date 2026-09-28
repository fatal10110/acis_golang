package effect

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// invulnerable is the effected surface the offensive landing refusal reads.
type invulnerable interface{ Invul() bool }

// damageDealer is the effector surface the offensive landing refusal reads.
type damageDealer interface{ CanGiveDamage() bool }

// Lands reports whether def's effects may land on effected from effector —
// the application-time gate every landing path shares. A self-application
// always passes. Otherwise effected must sit strictly inside def's effect
// range of effector (a target exactly at the range is outside it), and an
// offensive or debuff skill is refused against an invulnerable effected or
// from an effector not permitted to deal damage. A nil effector is
// sourceless: the range and damage-permission checks are skipped, the
// invulnerability refusal is not.
func Lands(effector, effected Actor, def modelskill.Definition) bool {
	if effector != nil && effector.ObjectID() == effected.ObjectID() {
		return true
	}
	if def.EffectRange > 0 && effector != nil && !inEffectRange(effector, effected, def.EffectRange) {
		return false
	}
	if !def.Offensive && !def.Debuff {
		return true
	}
	if inv, ok := effected.(invulnerable); ok && inv.Invul() {
		return false
	}
	if effector == nil {
		return true
	}
	dealer, ok := effector.(damageDealer)
	return !ok || dealer.CanGiveDamage()
}

func inEffectRange(effector, effected Actor, effectRange int) bool {
	ax, ay, az := effector.Position()
	bx, by, bz := effected.Position()
	return location.Location{X: ax, Y: ay, Z: az}.In3DRadius(location.Location{X: bx, Y: by, Z: bz}, effectRange)
}

// Attach names e's participants and hosts e on its owner's effect list: the
// effector's for a self-target kind, the effected's for every other kind.
// It reports false, leaving e unattached, when that owner is missing or has
// no effect list.
func Attach(e *Effect, effector, effected Actor) bool {
	owner := effected
	if e.SelfTarget {
		owner = effector
	}
	if owner == nil {
		return false
	}
	list := owner.EffectList()
	if list == nil {
		return false
	}
	e.Effector = effector
	e.Effected = effected
	list.Add(e)
	return true
}
