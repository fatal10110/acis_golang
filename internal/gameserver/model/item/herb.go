package item

// HerbPickup is one herb reward resulting from a herb-kind drop roll: the
// item a killer receives, and whether it was picked up directly rather
// than dropped to the ground for manual pickup.
type HerbPickup struct {
	ItemID   int32
	Amount   int32
	AutoLoot bool
}

// SplitHerbDrop turns one rolled herb drop (item and rolled quantity) into
// the pickups a killer receives. When auto-loot is enabled the killer
// always receives exactly one stack of the item, regardless of the rolled
// amount; otherwise every rolled unit becomes its own single-item pickup,
// since herbs never stack past 1 on the ground.
func SplitHerbDrop(itemID, amount int32, autoLoot bool) []HerbPickup {
	if amount <= 0 {
		return nil
	}
	if autoLoot {
		return []HerbPickup{{ItemID: itemID, Amount: 1, AutoLoot: true}}
	}
	pickups := make([]HerbPickup, amount)
	for i := range pickups {
		pickups[i] = HerbPickup{ItemID: itemID, Amount: 1}
	}
	return pickups
}
