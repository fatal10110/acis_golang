package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// Base inventory slot counts before the inventoryLimit stat, at the shipped
// players.properties defaults (MaximumSlotsForNoDwarf / MaximumSlotsForDwarf).
//
// ponytail: fixed at the shipped defaults; the config keys are not plumbed
// and the inventory's own SlotLimit does not use this yet (#2675). Upgrade
// when that issue wires player slot capacity.
const (
	baseInventoryLimit      = 80
	baseDwarfInventoryLimit = 100
)

// InventoryLimit returns how many item slots c's inventory may hold: the
// race's base count plus the inventoryLimit stat, truncated.
func (c *Character) InventoryLimit() int {
	base := baseInventoryLimit
	if c.Race == RaceDwarf {
		base = baseDwarfInventoryLimit
	}
	return base + int(c.CalcStat(stat.InvLim, 0))
}
