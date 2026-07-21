package player

import "time"

// ShortBuffUpdate is one short-buff HUD state change for the item-window
// healing-potion-family HUD slot: SkillID/Level/DurationSeconds for a new
// short buff, or the zero value to clear the HUD.
type ShortBuffUpdate struct {
	SkillID         int32
	Level           int32
	DurationSeconds int32
}

// SetShortBuffBroadcaster records the packet-layer hook UpdateShortBuff
// drives, both when a short buff starts and when its timer clears it.
func (c *Character) SetShortBuffBroadcaster(broadcast func(ShortBuffUpdate)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastShortBuff = broadcast
}

// ShortBuffTaskSkillID returns the skill id of the short buff currently
// showing on the item-window HUD slot, or 0 if none. Callers deciding
// whether a newly used skill should override the current HUD slot compare
// against this the way the reference does: only a numerically >= skill id
// takes over a running countdown.
func (c *Character) ShortBuffTaskSkillID() int32 {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.shortBuffTaskSkillID
}

// UpdateShortBuff starts (or restarts) the item-window short-buff HUD
// countdown for skillID/level, cancelling any timer already running for a
// previous short buff, and schedules an automatic clear broadcast once
// durationSeconds elapses. It unconditionally broadcasts and reschedules
// when called; gating which skills reach this call (the reference only
// calls its equivalent for the HP-potion skill family, and only when
// skillID is numerically >= ShortBuffTaskSkillID) is the caller's job.
func (c *Character) UpdateShortBuff(skillID, level, durationSeconds int32) {
	c.stateMu.Lock()
	if c.shortBuffTimer != nil {
		c.shortBuffTimer.Stop()
	}
	c.shortBuffTaskSkillID = skillID
	broadcast := c.broadcastShortBuff
	c.shortBuffTimer = time.AfterFunc(time.Duration(durationSeconds)*time.Second, c.clearShortBuff)
	c.stateMu.Unlock()

	if broadcast != nil {
		broadcast(ShortBuffUpdate{SkillID: skillID, Level: level, DurationSeconds: durationSeconds})
	}
}

// clearShortBuff resets the HUD slot and broadcasts its clear state; it
// runs on UpdateShortBuff's scheduled timer goroutine.
func (c *Character) clearShortBuff() {
	c.stateMu.Lock()
	c.shortBuffTaskSkillID = 0
	c.shortBuffTimer = nil
	broadcast := c.broadcastShortBuff
	c.stateMu.Unlock()

	if broadcast != nil {
		broadcast(ShortBuffUpdate{})
	}
}
