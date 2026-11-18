package festival

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// movesWorld is a fakeWorld whose players are all online; it records who
// was taken into an arena, settled and taken back out.
type movesWorld struct {
	*fakeWorld
	admitted, relocated []int32
	settles             int
}

func newMovesWorld() *movesWorld {
	return &movesWorld{fakeWorld: &fakeWorld{unspawns: map[NPC]int{}}}
}

func (w *movesWorld) Locate(int32) (Spot, bool)     { return Spot{}, true }
func (w *movesWorld) Admit(id int32, _, _, _ int)   { w.admitted = append(w.admitted, id) }
func (w *movesWorld) Settle(_ []int32, then func()) { w.settles++; then() }
func (w *movesWorld) Relocate(id int32, _ *Spot)    { w.relocated = append(w.relocated, id) }

// beginFestival starts the schedule with party signed up for Dawn's first
// festival and runs it until that festival begins. It returns the index in
// h.pending of the festival's first stage timer.
func (h *scheduleHarness) beginFestival(t *testing.T) int {
	t.Helper()
	h.m.Start()
	h.signUp(sevensigns.Dawn, 0, party)
	h.fire(t) // the first cycle
	h.fire(t) // the signup ends: the festival begins
	if !h.m.Initialized() {
		t.Fatal("the festival did not begin")
	}
	return len(h.pending) - 1
}

// A schedule replacing the one whose festival is still under way, with a
// signup period shorter than a festival, ends its first signup period as if
// nobody had signed up: no arena opens and the guides count a fresh
// signup period down. The festival under way then ends, taking its party
// back out, and leaves the replacing schedule alone.
func TestReplacingSignupEndsEmptyWhileTheFestivalRuns(t *testing.T) {
	h := newScheduleHarness(t)
	h.m.cfg.Length, h.m.cfg.CycleLength = 25*time.Minute, 40*time.Minute // a 14-minute signup
	w := newMovesWorld()
	h.m.SetWorld(w)
	festStage := h.beginFestival(t)
	if w.settles != 1 || len(w.admitted) != len(party) {
		t.Fatalf("festival begun: %d settles, admitted %v", w.settles, w.admitted)
	}

	h.m.CompetitionBegun()
	h.fire(t) // the replacing schedule's first cycle
	if h.lastDelay() != 14*time.Minute {
		t.Fatalf("replacing signup period = %v, want 14m", h.lastDelay())
	}
	before := len(h.pending)
	h.fire(t) // its signup ends while the festival is under way
	if w.settles != 1 || len(w.admitted) != len(party) {
		t.Fatalf("replacing signup end: %d settles, admitted %v; want no arena opened", w.settles, w.admitted)
	}
	if len(h.pending) != before+1 || h.lastDelay() != 26*time.Minute {
		t.Fatalf("replacing signup end: %d timers (was %d), next in %v; want the next signup period armed for 26m",
			len(h.pending), before, h.lastDelay())
	}
	if got := h.m.NextFestivalNotice(); got != notice("27") {
		t.Fatalf("notice after the replacing signup end = %q", got)
	}

	h.pending[festStage]() // the festival under way carries on
	for range 4 {          // the swarms, the second wave and the chests
		h.fire(t)
	}
	if !h.m.Initialized() {
		t.Fatal("the festival ended early")
	}
	armed := len(h.pending)
	h.fire(t) // the festival ends
	if h.m.Initialized() {
		t.Fatal("the festival did not end")
	}
	if !slices.Equal(w.relocated, []int32{1, 2, 3}) {
		t.Fatalf("taken back out = %v, want the party", w.relocated)
	}
	if len(h.pending) != armed {
		t.Fatalf("the festival's end armed %d timers", len(h.pending)-armed)
	}
}

// Once the schedule stops, a festival stage timer still pending does
// nothing: nothing spawns, nobody moves and no timer is armed.
func TestStopCallsOffTheFestivalUnderWay(t *testing.T) {
	h := newScheduleHarness(t)
	w := newMovesWorld()
	h.m.SetWorld(w)
	h.beginFestival(t)
	h.m.Stop()
	armed, admitted := len(h.pending), len(w.admitted)
	h.fire(t) // the stale first stage
	if len(w.spawned) != 0 || len(w.admitted) != admitted || len(w.relocated) != 0 || len(h.pending) != armed {
		t.Fatalf("stale stage after Stop: %d spawned, %d admitted, %d relocated, %d timers armed",
			len(w.spawned), len(w.admitted)-admitted, len(w.relocated), len(h.pending)-armed)
	}
}
