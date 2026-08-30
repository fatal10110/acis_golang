package player

import "math"

// TickRegen restores each resource below its current maximum once.
func (c *Character) TickRegen() {
	if c.AlikeDead() {
		return
	}
	changed := c.AddHP(math.Max(1, c.HPRegenRate())) > 0
	changed = c.AddMP(math.Max(1, c.MPRegenRate())) > 0 || changed
	changed = c.AddCP(math.Max(1, c.CPRegenRate())) > 0 || changed
	if changed {
		c.BroadcastMPStatus()
	}
}
