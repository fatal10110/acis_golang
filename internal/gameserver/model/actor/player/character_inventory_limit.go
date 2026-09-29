package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// InventorySlots is the configured base inventory slot count by race, before
// the inventoryLimit stat. Configured marks values read from
// players.properties, which are used as-is (an explicit 0 included); the
// zero value, a character never configured, uses the shipped default.
type InventorySlots struct {
	NoDwarf    int
	Dwarf      int
	Configured bool
}

// DefaultInventorySlots is the shipped players.properties base slot count.
var DefaultInventorySlots = InventorySlots{NoDwarf: 80, Dwarf: 100, Configured: true}

func (s InventorySlots) withDefaults() InventorySlots {
	if !s.Configured {
		return DefaultInventorySlots
	}
	return s
}

// InventoryLimit returns how many item slots c's inventory may hold: the
// configured base for c's race plus the inventoryLimit stat, truncated.
func (c *Character) InventoryLimit() int {
	c.stateMu.RLock()
	slots := c.inventorySlots.withDefaults()
	c.stateMu.RUnlock()
	base := slots.NoDwarf
	if c.Race == RaceDwarf {
		base = slots.Dwarf
	}
	return base + int(c.CalcStat(stat.InvLim, 0))
}
