package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// The NPC operations a monster behavior reads its fight by: its attack
// stance, its cast in flight and its root, whether it may attack a creature
// on its own, and the drop of one attack desire. Each is a hostile NPC's;
// any other NPC panics.

// MaxDriftRange is how far an NPC spawned at a point may stray from it
// before it is out of its territory.
const MaxDriftRange = npc.MaxDriftRange

// InCombat reports whether the NPC holds an attack stance.
func (n *NPC) InCombat() bool { return n.hostile("check its attack stance").InCombat() }

// CastingNow reports whether the NPC has a cast in flight.
func (n *NPC) CastingNow() bool { return n.hostile("check its cast").CastingNow() }

// Rooted reports whether an effect roots the NPC in place.
func (n *NPC) Rooted() bool { return n.hostile("check a root").Rooted() }

// CanAutoAttack reports whether the NPC may attack c on its own: c is alive
// and, unless the NPC already closes on it for a queued attack, passes the
// automatic target rule at the NPC's aggro range. Nil never passes.
func (n *NPC) CanAutoAttack(c Creature) bool {
	h := n.hostile("check an automatic attack")
	t := combatantOf(c)
	if t == nil {
		return false
	}
	return h.CanAutoAttack(t)
}

// RemoveAttackDesire zeroes the NPC's attack hate of c, drops its attack
// desire on c and stops its movement; the NPC goes back to peace when it
// hates no one any more. Nil only stops the movement.
func (n *NPC) RemoveAttackDesire(c Creature) {
	n.hostile("remove an attack desire").RemoveAttackDesire(combatantOf(c))
}

// Size returns the number of creatures the hate list holds.
func (l HateList) Size() int { return l.h.AI().Hates().Len() }
