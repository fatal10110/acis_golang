package player

// SetServitorVanishedNotifier records the packet-layer hook fired when this
// player's servitor is erased.
func (c *Character) SetServitorVanishedNotifier(send func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendServitorVanished = send
}

// ServitorVanished sends this player's YOUR_SERVITOR_HAS_VANISHED system
// message, matching Disablers.java's ERASE case
// (SystemMessageId.YOUR_SERVITOR_HAS_VANISHED).
func (c *Character) ServitorVanished() {
	c.stateMu.RLock()
	send := c.sendServitorVanished
	c.stateMu.RUnlock()
	if send != nil {
		send()
	}
}
