// Package sevensigns owns the Seven Signs event calendar: which of the four
// recurring periods is active in the current cycle, when it ends, and the
// persistence of that state across restarts. The period timeline is fixed:
//
//	RECRUITING -> COMPETITION -> RESULTS -> SEAL_VALIDATION -> RECRUITING (next cycle)
//
// The competition and validation periods end at 18:00 local time on the next
// Monday; recruiting and results are short intervals lasting fifteen minutes
// from the moment they begin.
//
// Only the calendar is implemented here. Cabals, seal ownership, stone and
// festival scores, period-change broadcasts, and dungeon teleports belong to
// the full Seven Signs port and are deliberately out of scope.
package sevensigns

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Period is one of the four phases a Seven Signs cycle passes through. The
// numeric order is the phase order; values wrap modulo periodCount.
type Period int

const (
	Recruiting Period = iota
	Competition
	Results
	SealValidation
	periodCount
)

// String returns the persisted enum name of p.
func (p Period) String() string {
	switch p {
	case Recruiting:
		return "RECRUITING"
	case Competition:
		return "COMPETITION"
	case Results:
		return "RESULTS"
	case SealValidation:
		return "SEAL_VALIDATION"
	default:
		return fmt.Sprintf("Period(%d)", int(p))
	}
}

// ParsePeriod parses a persisted enum name into p.
func ParsePeriod(name string) (Period, error) {
	for p := Period(0); p < periodCount; p++ {
		if p.String() == name {
			return p, nil
		}
	}
	return 0, fmt.Errorf("unknown seven signs period %q", name)
}

const (
	// A major period (competition or validation) ends at 18:00 local time.
	periodStartHour = 18
	periodStartMin  = 0
	// A minor period (recruiting or results) lasts fifteen minutes.
	periodMinorLength = 15 * time.Minute
)

// StatusRow is the persisted Seven Signs status: the current cycle number,
// the active period, and the moment the status was last written.
type StatusRow struct {
	Cycle    int
	Period   Period
	LastSave time.Time
}

// Store persists the single Seven Signs status row.
type Store interface {
	LoadStatus(ctx context.Context) (StatusRow, bool, error)
	SaveStatus(ctx context.Context, row StatusRow) error
}

// State tracks the active period and drives its transitions. All methods are
// safe for concurrent use.
type State struct {
	store     Store
	now       func() time.Time
	afterFunc func(time.Duration, func()) *time.Timer
	log       zerolog.Logger

	mu         sync.Mutex
	row        StatusRow
	nextChange time.Time
	timer      *time.Timer
}

// NewState returns a state persisting through store. now supplies wall time;
// afterFunc schedules the next transition timer (both overridden in tests;
// nil falls back to time.Now and time.AfterFunc).
func NewState(store Store, log zerolog.Logger, now func() time.Time, afterFunc func(time.Duration, func()) *time.Timer) *State {
	if now == nil {
		now = time.Now
	}
	if afterFunc == nil {
		afterFunc = time.AfterFunc
	}
	return &State{store: store, now: now, afterFunc: afterFunc, log: log}
}

// Restore loads the persisted status and computes when the active period
// ends. When the persisted save predates the moment the current period
// should have ended, the transition is marked due immediately: Start fires
// it without waiting. The database default row (cycle 1, competition, never
// saved) is assumed when none was written yet.
func (s *State) Restore(ctx context.Context) error {
	row, found, err := s.store.LoadStatus(ctx)
	if err != nil {
		return fmt.Errorf("load seven signs status: %w", err)
	}
	if !found {
		row = StatusRow{Cycle: 1, Period: Competition}
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.row = row
	if changeAlreadyDue(row, now) {
		s.nextChange = now
	} else {
		s.nextChange = nextPeriodChange(row.Period, now)
	}
	return nil
}

// Start arms the transition timer for the pending period change. Restore
// must have completed first.
func (s *State) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduleLocked()
}

// Stop cancels the pending transition timer.
func (s *State) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// CurrentPeriod returns the active period.
func (s *State) CurrentPeriod() Period {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.row.Period
}

// CurrentCycle returns the current cycle number.
func (s *State) CurrentCycle() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.row.Cycle
}

// NextChange reports when the active period ends.
func (s *State) NextChange() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextChange
}

// advance moves the state into the following period — starting the next
// cycle after validation ends — persists it, and re-arms the timer.
func (s *State) advance() {
	s.mu.Lock()
	ended := s.row.Period
	s.row.Period = (s.row.Period + 1) % periodCount
	if ended == SealValidation {
		s.row.Cycle++
	}
	s.row.LastSave = s.now()
	row := s.row
	s.nextChange = nextPeriodChange(s.row.Period, s.now())
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.store.SaveStatus(ctx, row); err != nil {
		s.log.Error().Err(err).Int("cycle", row.Cycle).Str("period", row.Period.String()).Msg("save seven signs status")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduleLocked()
}

func (s *State) scheduleLocked() {
	if s.timer != nil {
		s.timer.Stop()
	}
	delay := s.nextChange.Sub(s.now())
	if delay < 0 {
		delay = 0
	}
	s.timer = s.afterFunc(delay, s.advance)
}

// nextPeriodChange computes when period ends, given it is active at now. A
// major period ends at 18:00 local on the next Monday — today counts when it
// is Monday before 18:00. A minor period ends fifteen minutes after it
// began, measured from now: a restart during such a short interval grants it
// its full length again rather than resuming the previous countdown.
func nextPeriodChange(period Period, now time.Time) time.Time {
	switch period {
	case Competition, SealValidation:
		days := (int(time.Monday) - int(now.Weekday()) + 7) % 7
		if days == 0 && (now.Hour() > periodStartHour || (now.Hour() == periodStartHour && now.Minute() >= periodStartMin)) {
			days = 7
		}
		target := time.Date(now.Year(), now.Month(), now.Day(), periodStartHour, periodStartMin, 0, 0, now.Location())
		return target.AddDate(0, 0, days)
	default:
		return now.Add(periodMinorLength)
	}
}

// changeAlreadyDue reports whether the active period ended while the server
// was down, i.e. whether its scheduled end lies before the last status save.
// A never-saved row (the schema default) never qualifies.
func changeAlreadyDue(row StatusRow, now time.Time) bool {
	if row.LastSave.IsZero() || row.LastSave.UnixMilli() <= 7 {
		return false
	}
	var scheduledEnd time.Time
	switch row.Period {
	case Competition, SealValidation:
		offset := (int(now.Weekday()) - int(time.Monday) + 7) % 7
		thisMonday := time.Date(now.Year(), now.Month(), now.Day(), periodStartHour, periodStartMin, 0, 0, now.Location()).AddDate(0, 0, -offset)
		if now.Before(thisMonday) {
			thisMonday = thisMonday.AddDate(0, 0, -7)
		}
		scheduledEnd = thisMonday
	default:
		scheduledEnd = row.LastSave.Add(periodMinorLength)
	}
	return row.LastSave.Before(scheduledEnd)
}
