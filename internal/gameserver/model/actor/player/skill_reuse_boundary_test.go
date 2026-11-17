package player

import (
	"testing"
	"time"
)

func TestSkillDisabledHoldsThroughItsExpiryInstant(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	now := c.Now()

	c.disableSkillUntil(1, now)
	c.disableSkillUntil(2, now.Add(-time.Nanosecond))

	if !c.SkillDisabled(1) {
		t.Fatal("SkillDisabled() = false at the expiry instant, want true")
	}
	if c.SkillDisabled(2) {
		t.Fatal("SkillDisabled() = true once the clock passed the expiry, want false")
	}
}
