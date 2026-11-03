package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// The operations a siegable hall's treasure chest takes its blows by: it is
// invulnerable from the start, and the hit points of a barehanded blow come
// off through its own script instead.

// SetInvul makes the NPC invulnerable, or no longer so. Hits and harmful
// skills on an invulnerable NPC still reach its attacked hooks, but take no
// hit points. A handle on nothing does nothing.
func (n *NPC) SetInvul(v bool) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.SetInvul(v)
	case *npc.Folk:
		o.SetInvul(v)
	}
}

// SetHP sets the NPC's current hit points to hp, kept between 0 and its
// maximum, and refreshes the health bar of the players targeting it. A dead
// NPC keeps its hit points; a handle on nothing does nothing.
func (n *NPC) SetHP(hp float64) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.SetHP(hp)
	case *npc.Folk:
		o.SetHP(hp)
	}
}

// HoldsWeapon reports false: an NPC holds no item in its right hand.
func (n *NPC) HoldsWeapon() bool { return false }

// HoldsWeapon reports false: a door holds no item.
func (d *Door) HoldsWeapon() bool { return false }

// HoldsWeapon reports whether the player holds an item in its right hand.
func (p *Player) HoldsWeapon() bool { return p.RightHandItemID() != 0 }

// HoldsWeapon reports whether the creature holds an item in its right hand:
// only a pet does, from its own inventory; a servitor holds none.
func (c *creature) HoldsWeapon() bool {
	pet, ok := tracked(c).(inventoryPet)
	if !ok {
		return false
	}
	inv := pet.PetInventory()
	return inv != nil && inv.ItemAt(itemcontainer.RHand) != nil
}
