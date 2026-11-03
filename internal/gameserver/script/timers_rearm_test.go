package script

import (
	"strings"
	"testing"
	"time"
)

// armedTimer returns the timer name of the script labelled label, bound to
// nothing, and its sim timer.
func armedTimer(w *timerWorld, label, name string) *scriptTimer {
	own := w.script(label).ownTimers
	own.mu.Lock()
	defer own.mu.Unlock()
	st := own.loose[timerKey{name: name}]
	if st == nil {
		w.t.Fatalf("no timer %s of %s", name, label)
	}
	return st
}

// TestFixedRateTimerRearmsOnItsGrid: a fixed-rate timer fires on the grid of
// its first deadline through the one sim timer its start made; a hook that
// holds its queue past two deadlines has them fire at once, back to back,
// and the next one is back on the grid.
func TestFixedRateTimerRearmsOnItsGrid(t *testing.T) {
	w := newTimerWorld(t, listOf("quest.A"), func(w *timerWorld) Catalog {
		return Catalog{"quest.A": w.recorder("A", false)}
	})
	held := false
	w.onFire = func(*Script, Timer) {
		if !held {
			held = true
			w.clock.Advance(2500 * time.Millisecond)
		}
	}
	if !w.script("A").StartTimerAtFixedRate("tick", nil, nil, time.Second, time.Second) {
		t.Fatal("start refused")
	}
	st := armedTimer(w, "A", "tick")
	first := st.armed
	w.clock.Advance(5 * time.Second)

	want := []string{
		"fire A tick npc=none player=none at=1000",
		"fire A tick npc=none player=none at=3500",
		"fire A tick npc=none player=none at=3500",
		"fire A tick npc=none player=none at=4000",
		"fire A tick npc=none player=none at=5000",
	}
	if got := w.take(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("fired:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if armedTimer(w, "A", "tick") != st || st.armed != first {
		t.Fatal("a firing replaced the timer or its sim timer")
	}
}

// TestFixedRateTimerRearmAllocatesNothing: a firing of a fixed-rate timer
// whose hook answers nothing allocates nothing, its re-arm included.
func TestFixedRateTimerRearmAllocatesNothing(t *testing.T) {
	w := newTimerWorld(t, listOf("quest.A"), func(*timerWorld) Catalog {
		return Catalog{"quest.A": func() Script {
			return Script{Hooks: Hooks{OnTimer: func(*Script, Timer) string { return "" }}}
		}}
	})
	if !w.script("A").StartTimerAtFixedRate("tick", nil, nil, time.Second, time.Second) {
		t.Fatal("start refused")
	}
	w.clock.Advance(time.Second)
	if n := testing.AllocsPerRun(100, func() { w.clock.Advance(time.Second) }); n != 0 {
		t.Fatalf("a fixed-rate firing allocates %v times, want 0", n)
	}
}
