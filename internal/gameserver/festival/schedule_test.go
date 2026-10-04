package festival

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/rs/zerolog"
)

// fakeCalendar is a Seven Signs calendar set by hand.
type fakeCalendar struct {
	period     sevensigns.Period
	nextChange time.Time
}

func (c *fakeCalendar) CurrentPeriod() sevensigns.Period { return c.period }
func (c *fakeCalendar) NextChange() time.Time            { return c.nextChange }

// memStore keeps the festival rows in memory.
type memStore struct {
	scores []Score
	status Status
}

func (s *memStore) LoadScores(context.Context) ([]Score, error) { return s.scores, nil }
func (s *memStore) SaveScores(_ context.Context, scores []Score) error {
	s.scores = append([]Score(nil), scores...)
	return nil
}
func (s *memStore) LoadStatus(context.Context) (Status, bool, error) { return s.status, true, nil }
func (s *memStore) SaveStatus(_ context.Context, st Status) error {
	s.status = st
	return nil
}

// scheduleHarness runs a festival over a hand-set clock and calendar; each
// timer it arms is captured so the test fires it.
type scheduleHarness struct {
	now      time.Time
	calendar *fakeCalendar
	store    *memStore
	m        *Manager
	delays   []time.Duration
	pending  []func()
}

func newScheduleHarness(t *testing.T) *scheduleHarness {
	t.Helper()
	start := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	h := &scheduleHarness{
		now:      start,
		calendar: &fakeCalendar{period: sevensigns.Competition, nextChange: start.Add(5 * 24 * time.Hour)},
		store:    &memStore{status: Status{FestivalCycle: 4}},
	}
	h.m = New(DefaultConfig(), h.store, h.calendar, zerolog.Nop(), func() time.Time { return h.now }, func(d time.Duration, fn func()) *time.Timer {
		h.delays = append(h.delays, d)
		h.pending = append(h.pending, fn)
		return nil
	})
	if err := h.m.Restore(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	return h
}

// festivalCycle saves the status columns and returns the festival cycle
// written.
func (h *scheduleHarness) festivalCycle(t *testing.T) int {
	t.Helper()
	if err := h.m.SaveStatus(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h.store.status.FestivalCycle
}

// fire advances the clock by the last armed delay and runs that timer.
func (h *scheduleHarness) fire(t *testing.T) {
	t.Helper()
	if len(h.pending) == 0 {
		t.Fatal("no timer armed")
	}
	fn, d := h.pending[len(h.pending)-1], h.delays[len(h.delays)-1]
	h.now = h.now.Add(d)
	fn()
}

func (h *scheduleHarness) lastDelay() time.Duration { return h.delays[len(h.delays)-1] }

func notice(minutes string) string {
	return `<font color="FF0000">The next festival will begin in ` + minutes + ` minute(s).</font>`
}

// The default schedule (first cycle 2 minutes after the start, 38-minute
// cycles of which the festival takes 18 and the signup 19) counts a new
// festival cycle on start, awaits the first festival 2 + 19 minutes later,
// and, with nobody able to sign up, opens a new 19-minute signup period
// whenever one ends.
func TestScheduleCountsDownWithoutParties(t *testing.T) {
	h := newScheduleHarness(t)
	h.m.Start()
	if got := h.festivalCycle(t); got != 5 {
		t.Fatalf("festival cycle = %d, want 5", got)
	}
	if len(h.delays) != 1 || h.lastDelay() != 2*time.Minute {
		t.Fatalf("first timer = %v, want one of 2m", h.delays)
	}
	if got := h.m.NextFestivalNotice(); got != notice("22") {
		t.Fatalf("notice at start = %q", got)
	}
	// Thirty seconds on, 20.5 minutes are left: truncated to 20, plus one.
	h.now = h.now.Add(30 * time.Second)
	if got := h.m.NextFestivalNotice(); got != notice("21") {
		t.Fatalf("notice 30s in = %q", got)
	}
	h.now = h.now.Add(-30 * time.Second)

	h.fire(t) // the first cycle runs: the signup period lasts 19 minutes
	if h.lastDelay() != 19*time.Minute {
		t.Fatalf("signup timer = %v, want 19m", h.lastDelay())
	}
	if got := h.m.NextFestivalNotice(); got != notice("20") {
		t.Fatalf("notice as the cycle runs = %q", got)
	}
	h.fire(t) // the signup ends with nobody signed up
	if h.lastDelay() != 19*time.Minute {
		t.Fatalf("next signup timer = %v, want 19m", h.lastDelay())
	}
	if got := h.m.NextFestivalNotice(); got != notice("20") {
		t.Fatalf("notice after an empty signup = %q", got)
	}
	// Past the awaited start the count goes negative: -90s truncates to
	// -1, plus one.
	h.now = h.now.Add(19*time.Minute + 90*time.Second)
	if got := h.m.NextFestivalNotice(); got != notice("0") {
		t.Fatalf("notice past the start = %q", got)
	}
	if h.festivalCycle(t) != 5 {
		t.Fatal("an empty signup counted a festival cycle")
	}
}

// A cycle due during seal validation, or too close to the period's end
// for a whole cycle, is skipped: the next one is due a cycle length after
// it, and the awaited festival start is left as it was.
func TestScheduleSkipsCyclesItCannotFinish(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*fakeCalendar, time.Time)
	}{
		{"seal validation", func(c *fakeCalendar, _ time.Time) { c.period = sevensigns.SealValidation }},
		{"period ends within a cycle", func(c *fakeCalendar, now time.Time) { c.nextChange = now.Add(2*time.Minute + 37*time.Minute) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newScheduleHarness(t)
			h.m.Start()
			tc.set(h.calendar, h.now)
			h.fire(t)
			if h.lastDelay() != 38*time.Minute {
				t.Fatalf("retry timer = %v, want 38m", h.lastDelay())
			}
			h.calendar.period = sevensigns.Competition
			if got := h.m.NextFestivalNotice(); got != notice("20") {
				t.Fatalf("notice after a skipped cycle = %q", got)
			}
		})
	}
}

// The end of the competition stops the schedule: a timer already armed
// does nothing. The next competition starts a new schedule and counts a
// new festival cycle; Stop ends it for good.
func TestScheduleStopsWithTheCompetition(t *testing.T) {
	h := newScheduleHarness(t)
	h.m.Start()
	stale := h.pending[0]
	h.m.CompetitionEnded()
	before := len(h.pending)
	stale()
	if len(h.pending) != before {
		t.Fatal("a cancelled schedule's timer re-armed")
	}

	h.m.CompetitionBegun()
	if h.festivalCycle(t) != 6 || len(h.pending) != before+1 {
		t.Fatalf("restart: cycle %d, %d timers", h.festivalCycle(t), len(h.pending))
	}
	h.m.Stop()
	h.m.CompetitionBegun()
	if h.festivalCycle(t) != 6 || len(h.pending) != before+1 {
		t.Fatal("a stopped festival started a schedule")
	}
}

// During seal validation the schedule does not start, and the guides say
// the festivals resume next week.
func TestScheduleIdleDuringSealValidation(t *testing.T) {
	h := newScheduleHarness(t)
	h.calendar.period = sevensigns.SealValidation
	h.m.Start()
	if len(h.pending) != 0 || h.festivalCycle(t) != 4 {
		t.Fatalf("started during seal validation: %d timers, cycle %d", len(h.pending), h.festivalCycle(t))
	}
	want := `<font color="FF0000">This is the Seal Validation period. Festivals will resume next week.</font>`
	if got := h.m.NextFestivalNotice(); got != want {
		t.Fatalf("notice = %q", got)
	}
}
