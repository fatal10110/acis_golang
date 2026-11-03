package player

// FestivalRoster tells who is signed up for a Festival of Darkness. Any
// goroutine may call it.
type FestivalRoster interface {
	IsParticipant(objectID int32) bool
}

// SetFestivalRoster wires the festival roster c reads its participation
// from, called once by the network layer when it attaches c.
func (c *Character) SetFestivalRoster(r FestivalRoster) {
	c.festival.Store(&r)
}

// FestivalParticipant reports whether c is signed up for a Festival of
// Darkness. A character with no roster attached is not.
func (c *Character) FestivalParticipant() bool {
	p := c.festival.Load()
	return p != nil && *p != nil && (*p).IsParticipant(c.ObjectID())
}
