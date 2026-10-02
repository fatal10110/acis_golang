package player

// PartyRoom returns the id of the party-matching room the character is in,
// or 0. The party-matching registry sets it under its lock; the views other
// players see of the character read it.
func (c *Character) PartyRoom() int32 {
	return c.partyRoom.Load()
}

// SetPartyRoom records the party-matching room the character is in, 0 for
// none.
func (c *Character) SetPartyRoom(id int32) {
	c.partyRoom.Store(id)
}
