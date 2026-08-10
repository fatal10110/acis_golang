package player

import "time"

// chargeAutoClearDelay is how long Force/Soul charges survive without being
// spent or topped up before they auto-clear, matching the reference's
// 10-minute charge task.
const chargeAutoClearDelay = 10 * time.Minute

// Charges returns the current Force/Soul charge count.
func (c *Character) Charges() int {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.charges
}

// IncreaseCharges adds count charges, clamped to max, and restarts the
// auto-clear timer. It reports whether any charge was added; already being
// at max is a no-op that reports false, matching the reference's
// FORCE_MAXLEVEL_REACHED short-circuit.
func (c *Character) IncreaseCharges(count, max int) bool {
	c.stateMu.Lock()
	if c.charges >= max {
		send := c.sendChargeMessage
		charges := c.charges
		c.stateMu.Unlock()
		if send != nil {
			send(charges, true)
		}
		return false
	}
	c.charges += count
	maxed := c.charges >= max
	if maxed {
		c.charges = max
	}
	c.restartChargeTimerLocked()
	update := c.updateCharges
	send := c.sendChargeMessage
	charges := c.charges
	c.stateMu.Unlock()
	if send != nil {
		send(charges, maxed)
	}
	if update != nil {
		update()
	}
	return true
}

// SetChargesUpdater records the packet-layer hook that refreshes the Force/Soul charge display.
func (c *Character) SetChargesUpdater(update func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.updateCharges = update
}

// SetChargeMessageSender records the packet-layer hook that reports Force/Soul
// charge changes and the maximum-capacity outcome to the owning client.
func (c *Character) SetChargeMessageSender(send func(charges int, maxed bool)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendChargeMessage = send
}

// DecreaseCharges removes count charges, reporting whether there were
// enough to remove. Charges hitting zero stop the auto-clear timer instead
// of restarting it, matching the reference's stopChargeTask/restartChargeTask
// split.
func (c *Character) DecreaseCharges(count int) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.charges < count {
		return false
	}
	c.charges -= count
	if c.charges == 0 {
		c.stopChargeTimerLocked()
	} else {
		c.restartChargeTimerLocked()
	}
	return true
}

// ClearCharges resets the charge count to zero and cancels the auto-clear
// timer, matching the reference's clearCharges() called on death and
// subclass change.
func (c *Character) ClearCharges() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.charges = 0
	c.stopChargeTimerLocked()
}

func (c *Character) restartChargeTimerLocked() {
	c.stopChargeTimerLocked()
	log := c.log
	c.chargeTimer = time.AfterFunc(chargeAutoClearDelay, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("character: recovered panic in charge auto-clear callback")
			}
		}()
		c.ClearCharges()
	})
}

func (c *Character) stopChargeTimerLocked() {
	if c.chargeTimer != nil {
		c.chargeTimer.Stop()
		c.chargeTimer = nil
	}
}
