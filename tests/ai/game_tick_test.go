package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestGameTickMovesWithTheDrivenClock proves the game tick the scripts count
// (the whole seconds of the server's uptime) follows Advance: a tamed beast
// stamps the tick it came at, so one tamed 30 s after another holds a tick
// 30 later, and 1 s after that holds one more.
func TestGameTickMovesWithTheDrivenClock(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	w, master := bootTamer(t, nil, gameservertest.WithDrivenClockStart(start))
	if !w.srv.DrivesClock() {
		t.Skip("the real pool has no driven clock")
	}
	w.draws.set(99)

	first, _ := w.tame(t, master, ind8BuffaloID, 2188, 100)
	w.srv.Advance(t, 30*time.Second)
	second, _ := w.tame(t, master, ind8CougarID, 0, -100)
	w.srv.Advance(t, time.Second)
	third, _ := w.tame(t, master, ind8CougarID, 0, -200)

	a, b, c := ind8Slot(first, script.SlotIntAI1), ind8Slot(second, script.SlotIntAI1), ind8Slot(third, script.SlotIntAI1)
	if b-a != 30 || c-b != 1 {
		t.Fatalf("beast ticks %d, %d, %d after advances of 30 s and 1 s; want gaps of 30 and 1", a, b, c)
	}
}
