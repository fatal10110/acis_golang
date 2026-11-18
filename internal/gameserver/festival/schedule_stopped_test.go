package festival

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// A cycle under way as the competition ends keeps opening a signup period
// whenever one ends with nobody signed up: the guides count a fresh 1 to
// 20 minutes down through the results and, after seal validation, the
// recruiting period, until the next competition starts a new schedule.
func TestNoticeKeepsCountingFromCompetitionEndToNextCompetition(t *testing.T) {
	h := newScheduleHarness(t)
	h.m.Start()
	h.fire(t) // the first cycle runs
	h.fire(t) // an empty signup ends: the next festival is awaited 19 minutes on
	h.m.CompetitionEnded(func() {})

	h.calendar.period = sevensigns.Results
	if got := h.m.NextFestivalNotice(); got != notice("20") {
		t.Fatalf("notice as the results begin = %q", got)
	}
	h.fire(t) // the next signup ends: still nobody
	if h.lastDelay() != 19*time.Minute {
		t.Fatalf("signup timer in the results = %v, want 19m", h.lastDelay())
	}
	h.now = h.now.Add(15 * time.Minute)
	if got := h.m.NextFestivalNotice(); got != notice("5") {
		t.Fatalf("notice 15 minutes into a results signup = %q", got)
	}
	h.now = h.now.Add(-15 * time.Minute)

	h.calendar.period = sevensigns.SealValidation
	for range 7 * 24 * 60 / 19 {
		h.fire(t)
	}
	h.m.CycleBegun(2, func() {})
	h.calendar.period = sevensigns.Recruiting
	h.fire(t)
	if got := h.m.NextFestivalNotice(); got != notice("20") {
		t.Fatalf("notice in the recruiting after seal validation = %q", got)
	}

	h.m.CompetitionBegun()
	if got := h.m.NextFestivalNotice(); got != notice("22") {
		t.Fatalf("notice once the next competition begins = %q", got)
	}
	stale := len(h.pending) - 2 // the old cycle's signup timer
	before := len(h.pending)
	h.pending[stale]()
	if len(h.pending) != before {
		t.Fatal("the replaced cycle opened another signup period")
	}
}

// A cycle not yet begun as the competition ends is called off, as is the
// retry of a skipped one.
func TestCompetitionEndCallsOffCyclesNotBegun(t *testing.T) {
	h := newScheduleHarness(t)
	h.m.Start()
	h.calendar.period = sevensigns.SealValidation
	h.fire(t) // skipped: the next is due a cycle later
	h.calendar.period = sevensigns.Competition
	h.m.CompetitionEnded(func() {})
	before := len(h.pending)
	h.fire(t)
	if len(h.pending) != before {
		t.Fatal("a called-off cycle ran")
	}
}
