package task

import (
	"slices"
	"testing"
	"time"
)

// TestPvPFlagsRefreshWhileBlinkingWaitsForTheTick refreshes a blinking flag:
// the refresh only moves the deadline, the flag keeps blinking, and the
// next tick turns it steady again from the new deadline. Only an unflagged
// actor is flagged on at once.
func TestPvPFlagsRefreshWhileBlinkingWaitsForTheTick(t *testing.T) {
	now := time.UnixMilli(1_000)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	actor := &pvpFlagFakeActor{id: 1}

	flags.AddNormal(actor)
	now = now.Add(36 * time.Second)
	flags.Tick()
	testLoop.Run()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking}; !slices.Equal(got, want) {
		t.Fatalf("events before the refresh = %v, want %v", got, want)
	}

	flags.AddNormal(actor)
	flags.AddFlagged(actor)
	if got := actor.flag; got != PvPFlagBlinking {
		t.Fatalf("flag after refreshing a blinking flag = %v, want it still blinking", got)
	}
	flags.Tick()
	testLoop.Run()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking, PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("events after the tick = %v, want %v", got, want)
	}
}
