package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
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

// Creature is a script's handle on any creature: an NPC, a player, or
// another creature such as a summon.
type Creature interface {
	// ObjectID returns the creature's world object id, 0 for a handle on
	// nothing.
	ObjectID() int32
	// combatant is the creature as the world tracks it, nil for a handle on
	// nothing.
	combatant() attackable.Combatant
}

// creature is a handle on a creature that is neither an NPC nor a player.
type creature struct{ self attackable.Combatant }

// NewNPC returns a handle on the hostile NPC h.
func NewNPC(h *npc.Hostile) *NPC {
	return &NPC{self: h, brain: h.AI()}
}

// NPCOf returns the handle on the live NPC n, a hostile or a civilian
// NPC; nil for anything else.
func NPCOf(n attackable.Combatant) *NPC {
	switch o := n.(type) {
	case *npc.Hostile:
		return NewNPC(o)
	case *npc.Folk:
		// A civilian NPC's AI takes no desires yet (#3492): its handle
		// has no brain, and the registration gate of #3517 keeps desire
		// calls off it.
		return &NPC{self: o}
	}
	return nil
}

// PlayerOf returns the handle on the player p; nil for nil.
func PlayerOf(p attackable.Combatant) *Player {
	if p == nil {
		return nil
	}
	return &Player{self: p}
}

// creatureOf returns a handle on c: an NPC handle on an NPC, a player
// handle on a player, a creature handle otherwise, and nil for nil.
// A player is told by its kind, not its Go type: the world tracks a player
// wrapped with its connection, and the handle keeps that tracked value.
func creatureOf(c attackable.Combatant) Creature {
	if c == nil {
		return nil
	}
	if n := NPCOf(c); n != nil {
		return n
	}
	if c.Kind() == actor.KindPlayer {
		return PlayerOf(c)
	}
	return &creature{self: c}
}

// Decayed reports whether the NPC has left the world for good: its corpse
// decayed or it was deleted. A handle on nothing reports true.
func (n *NPC) Decayed() bool {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.Decayed()
	case *npc.Folk:
		return o.Decayed()
	}
	return true
}

// DeleteMe takes the NPC out of the world at once, leaving no corpse: its
// decayed hooks run, then its spawn answers as for a decayed corpse. It has
// left the world when the call returns. A handle on nothing, or on an NPC
// already gone, does nothing.
func (n *NPC) DeleteMe() {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.DeleteNow()
	case *npc.Folk:
		o.DeleteNow()
	}
}

// Summoner returns the creature the NPC was spawned for, nil for none.
func (n *NPC) Summoner() Creature {
	var inst *npc.Instance
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		inst = o.Instance
	case *npc.Folk:
		inst = o.Instance
	default:
		return nil
	}
	return creatureOf(inst.Summoner)
}

// NpcID returns the NPC's template id; a handle on nothing panics.
func (n *NPC) NpcID() int32 {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return int32(o.NpcID())
	case *npc.Folk:
		return int32(o.NpcID())
	}
	panic("script: the template of a handle on nothing")
}

// Alias returns the alias of the NPC's template, the key its walker routes
// are listed under; a handle on nothing panics.
func (n *NPC) Alias() string {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.Instance.Template.Alias
	case *npc.Folk:
		return o.Instance.Template.Alias
	}
	panic("script: the template of a handle on nothing")
}

// SetRunning puts the NPC in its run stance when run is true, in its walk
// stance otherwise; a change is shown to the players around it. A handle on
// nothing does nothing.
func (n *NPC) SetRunning(run bool) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		if run {
			o.ForceRunStance()
		} else {
			o.ForceWalkStance()
		}
	case *npc.Folk:
		o.SetRunStance(run)
	}
}

// Level returns the player's level.
func (p *Player) Level() int32 { return int32(p.character().Level()) }

// IsClanLeader reports whether the player leads its clan.
func (p *Player) IsClanLeader() bool { return p.character().IsClanLeader() }

// IsNoble reports whether the player is a noblesse.
func (p *Player) IsNoble() bool { return p.character().IsNoble() }

// Adena returns the adena the player carries.
func (p *Player) Adena() int32 {
	inv := p.character().Inventory()
	if inv == nil {
		return 0
	}
	return int32(inv.Adena())
}

// DeathPenaltyLevel returns the level of the player's death penalty, 0 for
// none.
func (p *Player) DeathPenaltyLevel() int32 { return int32(p.character().DeathPenaltyLevel()) }

// ReduceDeathPenaltyLevel lowers the player's death penalty by one level,
// telling the player the level left or that the penalty is lifted. A
// player with no death penalty is left as is, and so is a detaching one,
// which no script gives or takes anything.
func (p *Player) ReduceDeathPenaltyLevel() {
	if c := p.character(); !c.Detaching() {
		c.ReduceDeathPenaltyLevel()
	}
}

// ShowTeleportWindow opens on p the list of the destinations of kind the
// NPC offers, each priced for p. An NPC offering no destination shows
// nothing.
func (n *NPC) ShowTeleportWindow(p *Player, kind travel.Kind) {
	p.character().ShowTeleportWindow(n.ObjectID(), int(n.NpcID()), kind)
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

func (c *creature) combatant() attackable.Combatant {
	if c == nil {
		return nil
	}
	return c.self
}

// ObjectID returns the NPC's world object id.
func (n *NPC) ObjectID() int32 { return objectID(n) }

// ObjectID returns the player's world object id.
func (p *Player) ObjectID() int32 { return objectID(p) }

// ObjectID returns the creature's world object id.
func (c *creature) ObjectID() int32 { return objectID(c) }

func objectID(c Creature) int32 {
	if self := c.combatant(); self != nil {
		return self.ObjectID()
	}
	return 0
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
	AddFleeDesire(target attackable.Combatant, distance int, weight float64)
	AddSocialDesire(id, timer int, weight float64)
}

var (
	_ brain                = (*ai.Attackable)(nil)
	_ attackable.Combatant = (*npc.Hostile)(nil)
	_ attackable.Combatant = (*npc.Folk)(nil)
)

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

// AddFleeDesire asks the NPC to run distance away from target, counted from
// where it stands now. While it flees it takes up no other desire; reaching
// the end of the run drops the flee. Refused for an NPC that cannot move.
func (n *NPC) AddFleeDesire(target Creature, distance int, weight float64) {
	n.brain.AddFleeDesire(combatantOf(target), distance, weight)
}

// AddMoveRouteDesire asks the NPC to walk the walker route named route,
// listed under the NPC's template alias; a route with no node for it moves
// the NPC nowhere. The desire never loses weight and stays queued while the
// NPC lives, so the NPC walks the route whenever nothing outweighs it, and
// goes back to the node nearest to it once nothing does again.
func (n *NPC) AddMoveRouteDesire(route string, weight float64) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.AI().AddMoveRouteDesire(route, weight)
	case *npc.Folk:
		o.AddMoveRouteDesire(route, weight)
	}
}

// AddSocialDesire asks the NPC to play social animation id. Once it plays,
// the NPC takes up no other desire for timer milliseconds. Refused while the
// NPC's AI sleeps.
func (n *NPC) AddSocialDesire(id, timer int, weight float64) {
	n.brain.AddSocialDesire(id, timer, weight)
}
