package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// RestoreHennas loads equipped dyes from rows and recalculates bonuses for
// the character's current class.
func (c *Character) RestoreHennas(rows []henna.Row, lookup func(symbolID int) (henna.Henna, bool)) {
	if c.hennas == nil {
		c.hennas = &henna.List{}
	}
	c.hennas.Restore(rows, lookup)
	c.hennas.Recalculate(c.ClassID)
}

// HennaList returns the character's equipped-dye container, allocating an
// empty list on first use.
func (c *Character) HennaList() *henna.List {
	if c.hennas == nil {
		c.hennas = &henna.List{}
	}
	return c.hennas
}

// HennaSnapshot returns the HennaInfo payload for the current class.
func (c *Character) HennaSnapshot() henna.Snapshot {
	level, _ := ClassLevel(c.ClassID)
	return c.HennaList().Snapshot(c.ClassID, level)
}

// AddHenna equips h into the first empty slot allowed by class level.
func (c *Character) AddHenna(h henna.Henna) (dbSlot int, ok bool) {
	level, _ := ClassLevel(c.ClassID)
	return c.HennaList().Add(h, c.ClassID, level)
}

// RemoveHenna unequips the dye with symbolID.
func (c *Character) RemoveHenna(symbolID int) (dbSlot int, ok bool) {
	return c.HennaList().Remove(symbolID, c.ClassID)
}

// RefreshHennaStats recalculates dye bonuses after a class change without
// reloading rows from the database.
func (c *Character) RefreshHennaStats() {
	c.HennaList().Recalculate(c.ClassID)
}

func hennaBonusFor(c *Character, s stat.Stat) float64 {
	if c == nil || c.hennas == nil {
		return 0
	}
	switch s {
	case stat.StatINT:
		return float64(c.hennas.Stat(henna.StatINT))
	case stat.StatSTR:
		return float64(c.hennas.Stat(henna.StatSTR))
	case stat.StatCON:
		return float64(c.hennas.Stat(henna.StatCON))
	case stat.StatMEN:
		return float64(c.hennas.Stat(henna.StatMEN))
	case stat.StatDEX:
		return float64(c.hennas.Stat(henna.StatDEX))
	case stat.StatWIT:
		return float64(c.hennas.Stat(henna.StatWIT))
	default:
		return 0
	}
}
