package player

// SetMaxCpHpMp fills c's CP, MP and HP to their maxima and reports the
// change once. A dead character keeps its values and reports nothing.
func (c *Character) SetMaxCpHpMp() {
	res := c.ResourceValues()
	c.vitalsMu.Lock()
	if c.dead.Load() {
		c.vitalsMu.Unlock()
		return
	}
	c.curCP = res.MaxCP
	c.curMP = res.MaxMP
	c.writeHPLocked(res.MaxHP)
	c.vitalsMu.Unlock()
	c.BroadcastStatus()
}
