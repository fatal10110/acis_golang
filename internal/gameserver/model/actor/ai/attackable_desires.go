package ai

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// AddCastDesire queues a cast of ref at target with weight; an equal desire
// already queued gains the weight instead. Desire selection promotes it by
// weight like any other desire. Every refusal is silent:
//
//   - checkConditions refuses a skill still in reuse, or one whose MP or HP
//     cost the actor cannot pay; without it the request skips those gates.
//   - A hold (moveToTarget false) is refused when target is not strictly
//     within the skill's cast range plus both collision radii, measured
//     flat, since only walking would bring it in range.
//   - The desire aims at the creature ref's target type resolves from
//     target; a skill that resolves none, or that the actor cannot cast at
//     all, is refused.
//
// It takes no AI lock, so a hook may call it from any goroutine.
func (a *Attackable) AddCastDesire(target attackable.Combatant, ref skill.Ref, weight float64, checkConditions, moveToTarget bool) {
	cast := a.CastController()
	if target == nil || cast == nil {
		return
	}
	if checkConditions && !cast.CanDesire(target, ref) {
		return
	}
	if !moveToTarget && !a.inCastReach(target, cast.Range(ref)) {
		return
	}
	final := cast.FinalTarget(target, ref)
	if final == nil {
		return
	}
	a.desires.AddOrUpdate(&Desire{
		Kind:         IntentionCast,
		FinalTarget:  final,
		Skill:        ref,
		MoveToTarget: moveToTarget,
		Weight:       weight,
		QueuedAt:     a.now(),
	})
}

// inCastReach reports whether target stands strictly within castRange plus
// both collision radii of the actor, measured flat. The reach is truncated
// to whole units before the comparison.
func (a *Attackable) inCastReach(target attackable.Combatant, castRange int) bool {
	reach := int(float64(castRange) + a.actor.CollisionRadius() + target.CollisionRadius())
	ox, oy, oz := a.actor.Position()
	tx, ty, tz := target.Position()
	from := location.Location{X: ox, Y: oy, Z: oz}
	return from.Distance2D(location.Location{X: tx, Y: ty, Z: tz}) < float64(reach)
}

// AddWanderDesire queues a request to walk around the spawn territory with
// weight, its chain firing every timer seconds; an equal desire already
// queued gains the weight instead.
func (a *Attackable) AddWanderDesire(timer int, weight float64) {
	a.desires.AddOrUpdate(&Desire{
		Kind:     IntentionWander,
		Timer:    timer,
		Weight:   weight,
		QueuedAt: a.now(),
	})
}

// AddDoNothingDesire queues a request to keep still with weight; an equal
// desire already queued gains the weight instead. While it outweighs every
// other desire, desire selection keeps the actor doing nothing, which also
// keeps the idle follow and wander away. Its weight decays with the AI
// clock. timer is carried with the desire but times nothing.
func (a *Attackable) AddDoNothingDesire(timer int, weight float64) {
	a.desires.AddOrUpdate(&Desire{
		Kind:     IntentionNothing,
		Timer:    timer,
		Weight:   weight,
		QueuedAt: a.now(),
	})
}
