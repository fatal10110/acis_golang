package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// InventorySlots is the configured base inventory slot count by race, before
// the inventoryLimit stat. A zero field (a character never configured from
// players.properties) falls back to the shipped default.
type InventorySlots struct {
	NoDwarf int
	Dwarf   int
}

// DefaultInventorySlots is the shipped players.properties base slot count.
var DefaultInventorySlots = InventorySlots{NoDwarf: 80, Dwarf: 100}

func (s InventorySlots) withDefaults() InventorySlots {
	if s.NoDwarf == 0 {
		s.NoDwarf = DefaultInventorySlots.NoDwarf
	}
	if s.Dwarf == 0 {
		s.Dwarf = DefaultInventorySlots.Dwarf
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
