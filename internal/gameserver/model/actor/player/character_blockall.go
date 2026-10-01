package player

// BlockingAll reports whether the character blocks everything: every
// whisper, friend invitation and other request another player sends it is
// refused. It lasts for the login only. Other players' queues read it.
func (c *Character) BlockingAll() bool {
	return c.blockingAll.Load()
}

// SetBlockingAll turns the block-everything mode on or off.
func (c *Character) SetBlockingAll(on bool) {
	c.blockingAll.Store(on)
}
