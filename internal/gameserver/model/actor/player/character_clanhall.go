package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"

// HallFunctions answers the functions a clan hall rents.
type HallFunctions interface {
	// Level is the rented level of hall hallID's function of type
	// funcType, 0 when the hall rents none.
	Level(hallID int32, funcType int) int
}

// SetInClanHallZone records the live zone engine's current clan hall zone
// membership (zone.FlagClanHall).
func (c *Character) SetInClanHallZone(inside bool) {
	c.insideClanHallZone.Store(inside)
}

// InClanHallZone reports whether the character stands in a clan hall's
// grounds, any clan's.
func (c *Character) InClanHallZone() bool {
	return c.insideClanHallZone.Load()
}

// clanHallRegenMultiplier is what the recovery function of type funcType
// rented by the hall of c's clan multiplies c's regeneration by while c
// stands in any clan hall's grounds: 1 plus its level as a percentage, 1
// outside, without a clan hall or without the function.
func (c *Character) clanHallRegenMultiplier(funcType int) float64 {
	if !c.InClanHallZone() || c.hallFunctions == nil {
		return 1
	}
	hallID := c.ClanHallID()
	if hallID <= 0 {
		return 1
	}
	return 1 + float64(c.hallFunctions.Level(hallID, funcType))/100.0
}

// hpRegenHallMultiplier and mpRegenHallMultiplier are the clan hall HP and
// MP recovery bonuses.
func (c *Character) hpRegenHallMultiplier() float64 {
	return c.clanHallRegenMultiplier(clanhall.FuncRestoreHP)
}

func (c *Character) mpRegenHallMultiplier() float64 {
	return c.clanHallRegenMultiplier(clanhall.FuncRestoreMP)
}
