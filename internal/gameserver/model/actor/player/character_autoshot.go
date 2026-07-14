package player

// SetAutoSoulShot records whether itemID is active for automatic shot use.
func (c *Character) SetAutoSoulShot(itemID int32, enabled bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if enabled {
		if c.autoSoulShots == nil {
			c.autoSoulShots = make(map[int32]bool)
		}
		c.autoSoulShots[itemID] = true
		return
	}
	delete(c.autoSoulShots, itemID)
}

// AutoSoulShotEnabled reports whether itemID is active for automatic shot use.
func (c *Character) AutoSoulShotEnabled(itemID int32) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.autoSoulShots[itemID]
}
