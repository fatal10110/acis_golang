package summon

import "github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"

// WeaponItemID is the item id of the weapon a pet holds in its right hand,
// 0 for none. A servitor wears nothing.
func (a *Actor) WeaponItemID() int {
	return a.wornItemID(itemcontainer.RHand)
}

// ArmorItemID is the item id of the chest armor a pet wears, 0 for none. A
// servitor wears nothing.
func (a *Actor) ArmorItemID() int {
	return a.wornItemID(itemcontainer.Chest)
}

func (a *Actor) wornItemID(slot int) int {
	inv := a.PetInventory()
	if inv == nil {
		return 0
	}
	inst := inv.ItemAt(slot)
	if inst == nil {
		return 0
	}
	return int(inst.TemplateID)
}
