package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// WearingFormalWear reports whether the chest slot holds a full-body formal
// dress, which forbids item and skill use.
func (c *Character) WearingFormalWear() bool {
	inv := c.Inventory()
	if inv == nil {
		return false
	}
	inst := inv.ItemAt(itemcontainer.Chest)
	if inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	return ok && tmpl.Slot == item.SlotAllDress
}

// ActiveSiegeAttacker reports whether the character's clan attacks a siege
// running where the character stands. It is not wired to the castle sieges
// yet (#235), so a siege-summon skill always refuses.
func (c *Character) ActiveSiegeAttacker() bool { return false }
