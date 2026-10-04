package event

// DeathItemDrop reports that this player's death may cost it items: with
// Chance percent the death drops some, then each droppable item it holds
// is taken off if worn and drops with WeaponChance percent for a worn
// weapon, EquipChance for any other worn item and ItemChance for one not
// worn, until Limit items have dropped.
type DeathItemDrop struct {
	Chance       int
	EquipChance  int
	WeaponChance int
	ItemChance   int
	Limit        int
}

func (DeathItemDrop) event() {}
