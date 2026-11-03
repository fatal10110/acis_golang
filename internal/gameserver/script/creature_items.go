package script

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// A creature's own items, for a hook that asks whatever hit or reached its
// NPC what it carries. Only a player and a pet have an inventory; asking
// any other creature panics, so the invocation aborts at the question.

// HasAtLeastOneItem panics: an NPC has no inventory.
func (n *NPC) HasAtLeastOneItem(...int32) bool {
	panic(fmt.Sprintf("script: ask %T for items: it has no inventory", n.combatant()))
}

// HasAtLeastOneItem panics: a door has no inventory.
func (d *Door) HasAtLeastOneItem(...int32) bool {
	panic("script: ask a door for items: it has no inventory")
}

// inventoryPet is a summon that carries its own inventory: a pet.
type inventoryPet interface {
	PetInventory() *itemcontainer.Inventory
}

// HasAtLeastOneItem reports whether the pet's inventory holds an instance
// of any of itemIDs. A servitor, or any other creature, has no inventory
// and panics.
func (c *creature) HasAtLeastOneItem(itemIDs ...int32) bool {
	self := tracked(c)
	pet, ok := self.(inventoryPet)
	if !ok {
		panic(fmt.Sprintf("script: ask %T for items: it has no inventory", self))
	}
	inv := pet.PetInventory()
	if inv == nil {
		panic("script: ask a servitor for items: it has no inventory")
	}
	return inv.HasAnyItem(itemIDs...)
}
