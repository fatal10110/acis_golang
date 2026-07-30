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

// ConsumeHerb applies the herb itemID carries to this character and reports
// whether a consumer was there to apply it. A detached character has none, so
// the caller can still deliver the herb some other way instead of dropping it
// on the floor of a hook that no longer exists.
func (c *Character) ConsumeHerb(itemID int32) bool {
	c.stateMu.RLock()
	consume := c.consumeHerb
	c.stateMu.RUnlock()
	if consume == nil {
		return false
	}
	consume(itemID)
	return true
}
