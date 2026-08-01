package player

// IsPlayer identifies this actor as a player rather than another playable.
func (*Character) IsPlayer() bool { return true }

// SetRegenMaxSender records the packet-layer hook for a heal-over-time
// effect's client regen gauge.
func (c *Character) SetRegenMaxSender(send func(count, period int32, hpRegen float64)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendRegenMax = send
}

// SendRegenMax updates this player's client-only heal-over-time regen gauge.
func (c *Character) SendRegenMax(count, period int32, hpRegen float64) {
	c.stateMu.RLock()
	send := c.sendRegenMax
	c.stateMu.RUnlock()
	if send != nil {
		send(count, period, hpRegen)
	}
}
