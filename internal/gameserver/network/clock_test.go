package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

func TestGameClientLinkUsesConfiguredClock(t *testing.T) {
	clock := scheduler.NewManualClock(time.Unix(100, 0))
	link := NewGameClientLink(GameClientLinkConfig{Clock: clock})
	if link.clock != clock {
		t.Fatal("GameClientLink did not retain configured clock")
	}
}
