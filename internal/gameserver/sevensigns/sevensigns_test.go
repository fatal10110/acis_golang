package sevensigns

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func mustPeriod(t *testing.T, name string) Period {
	t.Helper()
	p, err := ParsePeriod(name)
	if err != nil {
		t.Fatalf("ParsePeriod(%q): %v", name, err)
	}
	return p
}

func TestPeriodStringAndParseRoundTrip(t *testing.T) {
	for _, p := range []Period{Recruiting, Competition, Results, SealValidation} {
		got, err := ParsePeriod(p.String())
		if err != nil || got != p {
			t.Fatalf("round trip %v: got (%v, %v)", p, got, err)
		}
	}
	if _, err := ParsePeriod("FESTIVAL"); err == nil {
		t.Fatal("ParsePeriod(unknown) = nil error")
	}
}

func at(year int, month time.Month, day, hour, min int, loc *time.Location) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, loc)
}

func TestNextPeriodChangeMajorEndsNextMondayEvening(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"monday morning", at(2026, time.August, 24, 10, 0, loc), at(2026, time.August, 24, 18, 0, loc)},
		{"monday evening exactly", at(2026, time.August, 24, 18, 0, loc), at(2026, time.August, 31, 18, 0, loc)},
		{"monday evening later", at(2026, time.August, 24, 21, 30, loc), at(2026, time.August, 31, 18, 0, loc)},
		{"midweek", at(2026, time.August, 26, 9, 0, loc), at(2026, time.August, 31, 18, 0, loc)},
		{"sunday", at(2026, time.August, 23, 23, 59, loc), at(2026, time.August, 24, 18, 0, loc)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, period := range []Period{Competition, SealValidation} {
				if got := nextPeriodChange(period, tc.now); !got.Equal(tc.want) {
					t.Fatalf("nextPeriodChange(%v, %s) = %s, want %s", period, tc.now, got, tc.want)
				}
			}
		})
	}
}

func TestNextPeriodChangeMinorLastsQuarterHour(t *testing.T) {
	now := at(2026, time.August, 25, 12, 7, time.UTC)
	for _, period := range []Period{Recruiting, Results} {
		if got := nextPeriodChange(period, now); !got.Equal(now.Add(15 * time.Minute)) {
			t.Fatalf("nextPeriodChange(%v, %s) = %s, want %s", period, now, got, now.Add(15*time.Minute))
		}
	}
}

func TestChangeAlreadyDue(t *testing.T) {
	now := at(2026, time.August, 26, 12, 0, time.UTC) // Wednesday
	thisMondayEvening := at(2026, time.August, 24, 18, 0, time.UTC)
	lastMondayEvening := at(2026, time.August, 17, 18, 0, time.UTC)

	cases := []struct {
		name string
		row  StatusRow
		now  time.Time
		want bool
	}{
		{"never saved", StatusRow{Cycle: 1, Period: Competition}, now, false},
		{"competition saved before this week's deadline", StatusRow{Period: Competition, LastSave: lastMondayEvening.Add(time.Hour)}, now, true},
		{"competition saved after this week's deadline", StatusRow{Period: Competition, LastSave: thisMondayEvening.Add(time.Hour)}, now, false},
		{"validation saved before the most recent monday evening", StatusRow{Period: SealValidation, LastSave: lastMondayEvening.Add(-time.Second)}, at(2026, time.August, 24, 10, 0, time.UTC), true},
		{"minor period with any save", StatusRow{Period: Results, LastSave: now.Add(-time.Second)}, now, true},
		{"recruiting with any save", StatusRow{Period: Recruiting, LastSave: now.Add(-time.Minute)}, now, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := changeAlreadyDue(tc.row, tc.now); got != tc.want {
				t.Fatalf("changeAlreadyDue(%+v, %s) = %v, want %v", tc.row, tc.now, got, tc.want)
			}
		})
	}
}

// recordingStore captures saves and serves one configured load.
type recordingStore struct {
	row      StatusRow
	found    bool
	saves    []StatusRow
	failSave bool
}

func (s *recordingStore) LoadStatus(context.Context) (StatusRow, bool, error) {
	return s.row, s.found, nil
}

func (s *recordingStore) SaveStatus(_ context.Context, row StatusRow) error {
	if s.failSave {
		return errors.New("db down")
	}
	s.saves = append(s.saves, row)
	return nil
}

// stateHarness builds a state over a controllable clock; advance captures
// each scheduled callback so tests fire transitions by hand.
type stateHarness struct {
	store   *recordingStore
	state   *State
	current time.Time
	fired   []func()
	delays  []time.Duration
}

func newStateHarness(t *testing.T, start time.Time) *stateHarness {
	t.Helper()
	h := &stateHarness{
		store:   &recordingStore{},
		current: start,
	}
	h.state = NewState(h.store, zerolog.Nop(), func() time.Time { return h.current }, func(d time.Duration, fn func()) *time.Timer {
		h.delays = append(h.delays, d)
		h.fired = append(h.fired, fn)
		return nil
	})
	return h
}

func TestRestoreMissingRowUsesSchemaDefaults(t *testing.T) {
	h := newStateHarness(t, at(2026, time.August, 26, 12, 0, time.UTC))
	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if h.state.CurrentPeriod() != Competition || h.state.CurrentCycle() != 1 {
		t.Fatalf("restored defaults = (cycle %d, %v)", h.state.CurrentCycle(), h.state.CurrentPeriod())
	}
}

func TestRestoreFreshCompetitionSchedulesNextMonday(t *testing.T) {
	start := at(2026, time.August, 26, 12, 0, time.UTC)
	h := newStateHarness(t, start)
	h.store.row = StatusRow{Cycle: 3, Period: Competition, LastSave: start.Add(-2 * time.Hour)}
	h.store.found = true

	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	h.state.Start()

	want := at(2026, time.August, 31, 18, 0, time.UTC)
	if !h.state.NextChange().Equal(want) {
		t.Fatalf("NextChange = %s, want %s", h.state.NextChange(), want)
	}
	if len(h.fired) != 1 || h.delays[0] != want.Sub(start) {
		t.Fatalf("schedule = (%d callbacks, first delay %s), want single %s", len(h.fired), h.delays[0], want.Sub(start))
	}
}

func TestOverdueMinorPeriodAdvancesOnStart(t *testing.T) {
	start := at(2026, time.August, 26, 12, 0, time.UTC)
	h := newStateHarness(t, start)
	h.store.row = StatusRow{Cycle: 2, Period: Results, LastSave: start.Add(-time.Hour)}
	h.store.found = true

	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !h.state.NextChange().Equal(start) {
		t.Fatalf("overdue change not marked due: %s", h.state.NextChange())
	}
	h.state.Start()
	if len(h.fired) != 1 || h.delays[0] != 0 {
		t.Fatalf("expected an immediate transition, got %d callbacks with delay %s", len(h.fired), h.delays[0])
	}

	h.current = start
	h.fired[0]()
	if h.state.CurrentPeriod() != SealValidation || h.state.CurrentCycle() != 2 {
		t.Fatalf("after advance = (cycle %d, %v)", h.state.CurrentCycle(), h.state.CurrentPeriod())
	}
	if len(h.store.saves) != 1 || h.store.saves[0].Period != SealValidation || h.store.saves[0].Cycle != 2 || !h.store.saves[0].LastSave.Equal(start) {
		t.Fatalf("saved rows = %+v", h.store.saves)
	}
	// The following validation period runs until the next Monday evening.
	if !h.state.NextChange().Equal(at(2026, time.August, 31, 18, 0, time.UTC)) {
		t.Fatalf("rescheduled to %s", h.state.NextChange())
	}
}

func TestAdvanceWrapsCycleAfterValidation(t *testing.T) {
	start := at(2026, time.August, 26, 12, 0, time.UTC)
	h := newStateHarness(t, start)
	h.store.row = StatusRow{Cycle: 4, Period: Results, LastSave: start.Add(-16 * time.Minute)}
	h.store.found = true

	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	h.state.Start()
	h.current = start.Add(time.Minute)
	h.fired[0]() // results -> seal validation
	h.fired[1]() // seal validation -> recruiting of cycle 5

	if h.state.CurrentPeriod() != Recruiting || h.state.CurrentCycle() != 5 {
		t.Fatalf("after wrap = (cycle %d, %v)", h.state.CurrentCycle(), h.state.CurrentPeriod())
	}
	if len(h.store.saves) != 2 {
		t.Fatalf("saves = %+v", h.store.saves)
	}
	last := h.store.saves[1]
	if last.Period != Recruiting || last.Cycle != 5 {
		t.Fatalf("final save = %+v", last)
	}
	// A fresh recruiting period lasts fifteen minutes from its start.
	if h.delays[len(h.delays)-1] != 15*time.Minute {
		t.Fatalf("recruiting reschedule = %s, want 15m0s", h.delays[len(h.delays)-1])
	}
}

func TestStopCancelsPendingTransition(t *testing.T) {
	start := at(2026, time.August, 26, 12, 0, time.UTC)
	h := newStateHarness(t, start)
	h.store.row = StatusRow{Cycle: 1, Period: Competition}
	h.store.found = true
	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	h.state.Start()
	h.state.Stop()
	// Nothing observable besides no panic; the timer handle was replaced by
	// a stub, so just verify no further scheduling happened implicitly.
	if len(h.fired) != 1 {
		t.Fatalf("callbacks fired = %d, want 1", len(h.fired))
	}
}

func TestFailedSaveKeepsCalendarRunning(t *testing.T) {
	start := at(2026, time.August, 26, 12, 0, time.UTC)
	h := newStateHarness(t, start)
	h.store.row = StatusRow{Cycle: 2, Period: Results, LastSave: start.Add(-time.Hour)}
	h.store.found = true
	h.store.failSave = true

	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	h.state.Start()
	h.fired[0]()
	if h.state.CurrentPeriod() != SealValidation {
		t.Fatalf("period after failed save = %v", h.state.CurrentPeriod())
	}
	if h.delays[len(h.delays)-1] != at(2026, time.August, 31, 18, 0, time.UTC).Sub(start) {
		t.Fatalf("no rescheduling after failed save: %s", h.delays[len(h.delays)-1])
	}
}
