package sim

import (
	"slices"
	"testing"
	"time"
)

// AdvanceBefore fires the timers due before the new time and leaves the ones
// due exactly at it armed, so a task posted at the new instant runs ahead of
// them on the next Advance.
func TestInlineAdvanceBeforeRunsPostedWorkAheadOfTimersDueThen(t *testing.T) {
	in := NewInline(epoch)
	q := in.NewQueue("q")
	var got []string
	q.After(50*time.Millisecond, func() { got = append(got, "early timer") })
	q.After(100*time.Millisecond, func() { got = append(got, "timer at the instant") })

	in.AdvanceBefore(100 * time.Millisecond)
	if want := []string{"early timer"}; !slices.Equal(got, want) {
		t.Fatalf("after AdvanceBefore ran %v, want %v", got, want)
	}
	if now := in.Now(); !now.Equal(epoch.Add(100 * time.Millisecond)) {
		t.Fatalf("Now() = %v, want the new time %v", now, epoch.Add(100*time.Millisecond))
	}
	q.Post(func() { got = append(got, "posted task") })
	in.Advance(0)
	if want := []string{"early timer", "posted task", "timer at the instant"}; !slices.Equal(got, want) {
		t.Fatalf("after Advance(0) ran %v, want %v", got, want)
	}
}
