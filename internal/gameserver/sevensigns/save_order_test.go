package sevensigns

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A Save requested while a period change's save is still writing must wait
// for it and then write its own, newer snapshot: the write that lands last
// carries the latest state, so a stone turn-in made during the change's
// save survives instead of being overwritten by the change's older row.
func TestSaveDuringTransitionSaveLandsLastWithNewerState(t *testing.T) {
	start := at(2026, time.August, 26, 12, 0, time.UTC)
	h := newStateHarness(t, start)
	h.store.row = StatusRow{Cycle: 3, Period: Recruiting, LastSave: start.Add(-time.Hour)}
	h.store.found = true
	ctx := context.Background()
	if err := h.state.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := h.state.SetPlayerInfo(ctx, 10, Dawn, Avarice); err != nil {
		t.Fatalf("SetPlayerInfo: %v", err)
	}
	h.state.Start()

	// Hold the change's status write open; later writes pass straight
	// through.
	var calls atomic.Int32
	saving := make(chan struct{})
	release := make(chan struct{})
	h.store.onSave = func() {
		if calls.Add(1) == 1 {
			close(saving)
			<-release
		}
	}
	changed := make(chan struct{})
	go func() {
		defer close(changed)
		h.fired[0]() // recruiting -> competition
	}()
	select {
	case <-saving:
	case <-time.After(5 * time.Second):
		t.Fatal("transition never reached its save window")
	}

	const blue = 10
	points, ok := h.state.AddPlayerStoneContrib(10, blue, 0, 0, 1_000_000)
	if !ok || points == 0 {
		t.Fatalf("AddPlayerStoneContrib = (%d, %v)", points, ok)
	}
	saved := make(chan error, 1)
	go func() { saved <- h.state.Save(ctx) }()

	// The second save cannot finish while the change's write is in flight;
	// give an unserialized save every chance to land first.
	select {
	case err := <-saved:
		close(release)
		<-changed
		t.Fatalf("Save finished (err %v) while the transition's save was still writing", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)

	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("transition did not finish after release")
	}
	select {
	case err := <-saved:
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Save did not finish after the transition's save")
	}

	if len(h.store.saves) != 2 || len(h.store.playerSaves) != 2 {
		t.Fatalf("writes = %d status, %d players; want 2 each", len(h.store.saves), len(h.store.playerSaves))
	}
	last := h.store.saves[1]
	if last.Period != Competition || last.DawnStoneScore != float64(points) {
		t.Fatalf("last status row = (period %v, dawn stones %v), want (competition, %d)", last.Period, last.DawnStoneScore, points)
	}
	lastPlayers := h.store.playerSaves[1]
	if len(lastPlayers) != 1 || lastPlayers[0].BlueStones != blue || lastPlayers[0].ContributionScore != points || lastPlayers[0].AncientAdena != points {
		t.Fatalf("last player rows = %+v, want the turn-in of %d blue stones", lastPlayers, blue)
	}
}
