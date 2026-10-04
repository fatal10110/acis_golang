package festival

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Once the competition ends the awaited festival start no longer moves:
// the guides count down past zero through the results period and, in the
// recruiting period after seal validation, show about a week's worth of
// minutes below zero until the next competition starts a new schedule.
func TestNoticeFrozenFromCompetitionEndToNextCompetition(t *testing.T) {
	h := newScheduleHarness(t)
	h.m.Start()
	h.fire(t) // the first cycle runs
	h.fire(t) // an empty signup ends: the next festival is awaited 19 minutes on
	h.m.CompetitionEnded()

	h.calendar.period = sevensigns.Results
	if got := h.m.NextFestivalNotice(); got != notice("20") {
		t.Fatalf("notice as the results begin = %q", got)
	}
	h.now = h.now.Add(15 * time.Minute)
	if got := h.m.NextFestivalNotice(); got != notice("5") {
		t.Fatalf("notice at the end of the results = %q", got)
	}

	h.calendar.period = sevensigns.SealValidation
	h.now = h.now.Add(7 * 24 * time.Hour)
	h.m.CycleBegun(2)
	h.calendar.period = sevensigns.Recruiting
	// 19 - 15 - 10080 minutes left, plus one.
	if got := h.m.NextFestivalNotice(); got != notice("-10075") {
		t.Fatalf("notice in the recruiting after seal validation = %q", got)
	}

	h.m.CompetitionBegun()
	if got := h.m.NextFestivalNotice(); got != notice("22") {
		t.Fatalf("notice once the next competition begins = %q", got)
	}
}
