package festival

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// A boot in the recruiting period starts the schedule, then the overdue
// recruiting end begins the competition at once: the schedule it replaces
// is cancelled first, so its pending first cycle does nothing and one
// schedule runs (parity exception, #5024). A festival already under way
// when its schedule is replaced runs to its end and takes its party back
// out: TestReplacingSignupEndsEmptyWhileTheFestivalRuns.
func TestBootInRecruitingRunsOneSchedule(t *testing.T) {
	h := newScheduleHarness(t)
	h.calendar.period = sevensigns.Recruiting
	h.m.Start()
	h.m.CompetitionBegun()
	if len(h.pending) != 2 {
		t.Fatalf("timers armed by the boot and the competition start = %d, want 2", len(h.pending))
	}
	if got := h.festivalCycle(t); got != 6 {
		t.Fatalf("festival cycle = %d, want 6: counted by each start", got)
	}

	h.now = h.now.Add(h.delays[0])
	h.pending[0]() // the replaced schedule's first cycle comes due
	if len(h.pending) != 2 {
		t.Fatalf("the replaced schedule armed %d timers", len(h.pending)-2)
	}
	h.fire(t) // the running schedule's first cycle
	if len(h.pending) != 3 || h.lastDelay() != 19*time.Minute {
		t.Fatalf("running schedule's first cycle: %d timers, next in %v; want one signup period of 19m", len(h.pending), h.lastDelay())
	}
	if got := h.festivalCycle(t); got != 6 {
		t.Fatalf("festival cycle after the first cycles = %d, want 6", got)
	}
}
