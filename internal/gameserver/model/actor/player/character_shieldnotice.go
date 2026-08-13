package player

// SetShieldBlockNotifiers records the packet-layer hooks for a live
// shield-block roll's client feedback: onSuccess fires for a normal block,
// onPerfect for the unconditional perfect block (Formulas.java:866-879).
func (c *Character) SetShieldBlockNotifiers(onSuccess, onPerfect func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendShieldBlockSuccess = onSuccess
	c.sendShieldBlockPerfect = onPerfect
}
