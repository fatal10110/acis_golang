package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

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
		charges := c.charges
		c.stateMu.Unlock()
		c.emit(event.ChargeMessage{Charges: charges, Maxed: true})
		return false
	}
	c.charges += count
	maxed := c.charges >= max
	if maxed {
		c.charges = max
	}
	c.restartChargeTimerLocked()
	charges := c.charges
	c.stateMu.Unlock()
	c.emit(event.ChargeMessage{Charges: charges, Maxed: maxed})
	c.emit(event.ChargesChanged{})
	return true
}

// DecreaseCharges removes count charges, reporting whether there were
// enough to remove. Charges hitting zero stop the auto-clear timer instead
// of restarting it, matching the reference's stopChargeTask/restartChargeTask
// split.
func (c *Character) DecreaseCharges(count int) bool {
	c.stateMu.Lock()
	if c.charges < count {
		c.stateMu.Unlock()
		return false
	}
	c.charges -= count
	if c.charges == 0 {
		c.stopChargeTimerLocked()
	} else {
		c.restartChargeTimerLocked()
	}
	c.stateMu.Unlock()
	c.emit(event.ChargesChanged{})
	return true
}

// ClearCharges resets the charge count to zero and cancels the auto-clear
// timer, matching the reference's clearCharges() called on death and
// subclass change.
func (c *Character) ClearCharges() {
	c.stateMu.Lock()
	changed := c.charges > 0
	c.charges = 0
	c.stopChargeTimerLocked()
	c.stateMu.Unlock()
	if changed {
		c.emit(event.ChargesChanged{})
	}
}

func (c *Character) restartChargeTimerLocked() {
	c.stopChargeTimerLocked()
	c.chargeTimer = c.afterLocked(chargeAutoClearDelay, c.ClearCharges)
}

func (c *Character) stopChargeTimerLocked() {
	if c.chargeTimer != nil {
		c.chargeTimer.Stop()
		c.chargeTimer = nil
	}
}
