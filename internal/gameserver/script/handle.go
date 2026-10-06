package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// NPC is a script's handle on one NPC.
type NPC struct {
	// self is the NPC as the world tracks it.
	self attackable.Combatant
	// brain is the AI the NPC's desires are queued on.
	brain brain
}

// Player is a script's handle on one player.
type Player struct {
	// self is the player as the world tracks it.
	self attackable.Combatant
}

// Creature is a script's handle on any creature: an NPC or a player.
type Creature interface {
	// combatant is the creature as the world tracks it, nil for a handle on
	// nothing.
	combatant() attackable.Combatant
}

func (n *NPC) combatant() attackable.Combatant {
	if n == nil || n.self == nil {
		return nil
	}
	return n.self
}

func (p *Player) combatant() attackable.Combatant {
	if p == nil || p.self == nil {
		return nil
	}
	return p.self
}

// combatantOf resolves c to the creature the world tracks, nil when c is a
// handle on nothing.
func combatantOf(c Creature) attackable.Combatant {
	if c == nil {
		return nil
	}
	return c.combatant()
}

// brain is the AI an NPC handle queues desires on: a hostile NPC's.
type brain interface {
	AddAttackDesire(target attackable.Combatant, weight float64)
	AddAttackDesireHold(target attackable.Combatant, weight float64)
	AddAttackDesireDamage(target attackable.Combatant, damage int, weight float64)
	AddCastDesire(target attackable.Combatant, ref skill.Ref, weight float64, checkConditions, moveToTarget bool)
	AddFollowDesire(target attackable.Combatant, weight float64)
	AddWanderDesire(timer int, weight float64)
	AddDoNothingDesire(timer int, weight float64)
}

var _ brain = (*ai.Attackable)(nil)

// Every desire below is ranked by weight against the NPC's other desires,
// and the heaviest is acted on at the NPC's next think. A request equal to
// one already queued adds its weight to it. A request on a handle on
// nothing is dropped.

// AddAttackDesire asks the NPC to attack target, closing in on it, and adds
// weight to the NPC's hate of target. An NPC that hates no one yet thinks at
// once.
func (n *NPC) AddAttackDesire(target Creature, weight float64) {
	n.brain.AddAttackDesire(combatantOf(target), weight)
}

// AddAttackDesireHold is AddAttackDesire for an NPC that stays where it is:
// it never walks toward target.
func (n *NPC) AddAttackDesireHold(target Creature, weight float64) {
	n.brain.AddAttackDesireHold(combatantOf(target), weight)
}

// AddAttackDesireDamage is AddAttackDesire that also counts damage as dealt
// to the NPC by target.
func (n *NPC) AddAttackDesireDamage(target Creature, damage int, weight float64) {
	n.brain.AddAttackDesireDamage(combatantOf(target), damage, weight)
}

// AddCastDesire asks the NPC to cast ref at target, closing in on it. It is
// refused when ref is still in reuse or its MP or HP cost cannot be paid.
// The cast aims at the creature ref's target type picks from target.
func (n *NPC) AddCastDesire(target Creature, ref skill.Ref, weight float64) {
	n.brain.AddCastDesire(combatantOf(target), ref, weight, true, true)
}

// AddCastDesireUnchecked is AddCastDesire without the reuse and cost
// checks.
func (n *NPC) AddCastDesireUnchecked(target Creature, ref skill.Ref, weight float64) {
	n.brain.AddCastDesire(combatantOf(target), ref, weight, false, true)
}

// AddCastDesireHold is AddCastDesire for an NPC that stays where it is: it
// is also refused when target stands out of ref's reach from where the NPC
// is.
func (n *NPC) AddCastDesireHold(target Creature, ref skill.Ref, weight float64) {
	n.brain.AddCastDesire(combatantOf(target), ref, weight, true, false)
}

// AddFollowDesire asks the NPC to follow target.
func (n *NPC) AddFollowDesire(target Creature, weight float64) {
	n.brain.AddFollowDesire(combatantOf(target), weight)
}

// AddWanderDesire asks the NPC to walk around its spawn territory, trying a
// walk every timer seconds.
func (n *NPC) AddWanderDesire(timer int, weight float64) {
	n.brain.AddWanderDesire(timer, weight)
}

// AddDoNothingDesire asks the NPC to keep still while the desire outweighs
// the rest. Its weight decays as the NPC's AI runs; timer times nothing.
func (n *NPC) AddDoNothingDesire(timer int, weight float64) {
	n.brain.AddDoNothingDesire(timer, weight)
}
