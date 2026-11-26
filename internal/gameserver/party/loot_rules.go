package party

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// LootMember is a member the loot rules weigh: its body, whether it is
// dead or leaving the world, and the inventory loot would go into (nil
// for none).
type LootMember interface {
	Body
	Dead() bool
	Detaching() bool
	Inventory() *itemcontainer.Inventory
}

// LootEligible accepts a member that may take units of itemID from origin
// under a random or by-turn loot rule: alive, not leaving the world, with
// room for all of them, and within partyRange of origin, body to body in
// 3D. units is how many a non-stackable arrives as: the whole count under
// the multiple item drop rule, else one (#5098).
func LootEligible[M LootMember](partyRange int, itemID int32, units int, origin LootOrigin) func(M) bool {
	return func(m M) bool {
		inv := m.Inventory()
		return !m.Dead() && !m.Detaching() && inv != nil && inv.ValidateCapacityByItemID(itemID, units) && InRange(partyRange, origin, m)
	}
}

// AdenaShares splits adena evenly among the members within partyRange of
// origin whose adena is not already at its cap and that are not leaving
// the world. takers keeps the members' order; the remainder of the
// division is lost, so share may be 0.
func AdenaShares[M LootMember](members []M, adena int, origin LootOrigin, partyRange int) (takers []M, share int) {
	for _, m := range members {
		inv := m.Inventory()
		if inv == nil || m.Detaching() || inv.Adena() == math.MaxInt32 || !InRange(partyRange, origin, m) {
			continue
		}
		takers = append(takers, m)
	}
	if len(takers) == 0 {
		return nil, 0
	}
	return takers, adena / len(takers)
}
