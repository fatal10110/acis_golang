package player

// ClanCastleID is the castle c's clan owns now, 0 when c has no clan or
// its clan owns none.
func (c *Character) ClanCastleID() int32 {
	clanID := c.ClanID()
	if clanID == 0 || c.social == nil {
		return 0
	}
	return c.social.ClanCastleID(clanID)
}

// ClanHallID is the clan hall c's clan owns now, 0 when c has no clan or
// its clan owns none.
func (c *Character) ClanHallID() int32 {
	clanID := c.ClanID()
	if clanID == 0 || c.social == nil {
		return 0
	}
	return c.social.ClanHallID(clanID)
}
