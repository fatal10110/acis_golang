package player

// SetHerbConsumer records the hook that applies a received herb's carried
// skill to this character. Herbs never enter an inventory: whatever hands a
// herb to a player consumes it on the spot, and the packet layer owns the
// cast broadcast and effect application.
func (c *Character) SetHerbConsumer(consume func(itemID int32)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.consumeHerb = consume
}

// ConsumeHerb applies the herb itemID carries to this character. It is a
// no-op when no consumer is wired, the way a herb with no registered item
// handler is simply discarded.
func (c *Character) ConsumeHerb(itemID int32) {
	c.stateMu.RLock()
	consume := c.consumeHerb
	c.stateMu.RUnlock()
	if consume != nil {
		consume(itemID)
	}
}
