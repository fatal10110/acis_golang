package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// StorageSlots is the configured base size of every per-player storage the
// character does not carry on itself, before each one's limit stat.
// Configured marks values read from players.properties, which are used as-is
// (an explicit 0 included); the zero value, a character never configured,
// uses the shipped defaults.
type StorageSlots struct {
	WarehouseNoDwarf    int
	WarehouseDwarf      int
	Freight             int
	PrivateStoreNoDwarf int
	PrivateStoreDwarf   int
	DwarfRecipe         int
	CommonRecipe        int
	Configured          bool
}

// DefaultStorageSlots is the shipped players.properties storage sizes.
var DefaultStorageSlots = StorageSlots{
	WarehouseNoDwarf:    100,
	WarehouseDwarf:      120,
	Freight:             20,
	PrivateStoreNoDwarf: 4,
	PrivateStoreDwarf:   5,
	DwarfRecipe:         50,
	CommonRecipe:        50,
	Configured:          true,
}

func (s StorageSlots) withDefaults() StorageSlots {
	if !s.Configured {
		return DefaultStorageSlots
	}
	return s
}

func (c *Character) configuredStorageSlots() StorageSlots {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.storageSlots.withDefaults()
}

// byRace picks the dwarf or non-dwarf base for c.
func (c *Character) byRace(noDwarf, dwarf int) int {
	if c.Race == RaceDwarf {
		return dwarf
	}
	return noDwarf
}

// limitStat returns the truncated sum of c's st bonuses.
func (c *Character) limitStat(st stat.Stat) int {
	return int(c.CalcStat(st, 0))
}

// WarehouseLimit returns how many item slots c's private warehouse may
// hold: the configured base for c's race plus the whLimit stat.
func (c *Character) WarehouseLimit() int {
	s := c.configuredStorageSlots()
	return c.byRace(s.WarehouseNoDwarf, s.WarehouseDwarf) + c.limitStat(stat.WhLim)
}

// FreightLimit returns how many item slots c's freight may hold: the
// configured base plus the FreightLimit stat.
func (c *Character) FreightLimit() int {
	return c.configuredStorageSlots().Freight + c.limitStat(stat.FreightLim)
}

// PrivateSellStoreLimit returns how many rows c's private sell store may
// list: the configured base for c's race plus the PrivateSellLimit stat.
func (c *Character) PrivateSellStoreLimit() int {
	s := c.configuredStorageSlots()
	return c.byRace(s.PrivateStoreNoDwarf, s.PrivateStoreDwarf) + c.limitStat(stat.PSellLim)
}

// PrivateBuyStoreLimit returns how many rows c's private buy store may
// list: the configured base for c's race plus the PrivateBuyLimit stat.
func (c *Character) PrivateBuyStoreLimit() int {
	s := c.configuredStorageSlots()
	return c.byRace(s.PrivateStoreNoDwarf, s.PrivateStoreDwarf) + c.limitStat(stat.PBuyLim)
}

// DwarfRecipeLimit returns how many dwarven recipes c's recipe book may
// hold: the configured base plus the DwarfRecipeLimit stat.
func (c *Character) DwarfRecipeLimit() int {
	return c.configuredStorageSlots().DwarfRecipe + c.limitStat(stat.RecDLim)
}

// CommonRecipeLimit returns how many common recipes c's recipe book may
// hold: the configured base plus the CommonRecipeLimit stat.
func (c *Character) CommonRecipeLimit() int {
	return c.configuredStorageSlots().CommonRecipe + c.limitStat(stat.RecCLim)
}
