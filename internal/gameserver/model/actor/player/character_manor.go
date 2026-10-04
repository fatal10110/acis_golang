package player

// InPartyWith reports whether the player playerID is in c's party, the way
// a crop's sower shares its harvest.
func (c *Character) InPartyWith(playerID int32) bool {
	return c.social != nil && c.social.SameParty(c.ID, playerID)
}
