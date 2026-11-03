package event

// DeathItemDrop reports that this player's death may cost it items: with
// Chance percent the death drops some, then each droppable item it holds
// is taken off if worn and drops with WeaponChance percent for a worn
// weapon, EquipChance for any other worn item and ItemChance for one not
// worn, until Limit items have dropped. A game master drops nothing unless
// GMDrops is set, and no item whose template is in Kept ever drops. All of
// it comes from the one set of rules the player's death rolled under.
type DeathItemDrop struct {
	Chance       int
	EquipChance  int
	WeaponChance int
	ItemChance   int
	Limit        int
	GMDrops      bool
	Kept         []int32
}

func (DeathItemDrop) event() {}
