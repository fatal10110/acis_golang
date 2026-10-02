// Package olympiad owns the Olympiad calendar and the nobles' records.
//
// An Olympiad runs until noon on the first day of the next month. Inside it
// each day has a competition window, from the configured start time for the
// configured length, and a validation period filling the rest of the day.
// Every competition start opens a new cycle and clears the nobles'
// records; a weekly grant adds points to every record; and the end of the
// Olympiad keeps a copy of the records as the month's standings, though a
// period change due at the same moment always runs first and moves the end
// a month on. The cycle number and the records persist across restarts.
// Whether the daily new cycle and the unreachable end should stay is open
// (#3280).
//
// The matches themselves and the heroes they elect are not part of this
// package yet: the competition window starts no game manager (#217) and the
// end of an Olympiad elects no heroes (#220).
package olympiad

import (
	"context"
	"maps"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// TaskTimeout bounds the database writes of one calendar step, and of the
// save on Stop.
const TaskTimeout = 10 * time.Second

// Period is the part of the day the Olympiad is in.
type Period int

const (
	// Competition is the daily match window.
	Competition Period = iota
	// Validation is the rest of the day, between two competition windows.
	Validation
)

// Config is the Olympiad's configuration.
type Config struct {
	// StartHour and StartMinute are the local time each day's competition
	// window opens at.
	StartHour, StartMinute int
	// CompetitionMillis is the length of the competition window.
	CompetitionMillis int64
	// WeeklyPoints is added to every noble's points each week.
	WeeklyPoints int
}

// DefaultConfig returns the configuration used when no setting overrides
// it: a six-hour window opening at 18:00, three weekly points.
func DefaultConfig() Config {
	return Config{StartHour: 18, CompetitionMillis: 21600000, WeeklyPoints: 3}
}

// Notice is a calendar change announced to every player online.
type Notice int

const (
	// NoticeCompetitionStarted: a competition window opened.
	NoticeCompetitionStarted Notice = iota
	// NoticeCompetitionEnded: a competition window closed.
	NoticeCompetitionEnded
	// NoticeRegistrationEnded: registration for this window closed.
	NoticeRegistrationEnded
	// NoticeCycleStarted: a new cycle, numbered by the announcement, began.
	NoticeCycleStarted
	// NoticeCycleEnded: the cycle numbered by the announcement ended.
	NoticeCycleEnded
)

// Announcer tells every player online about a calendar change. cycle is the
// cycle number for NoticeCycleStarted and NoticeCycleEnded, zero otherwise.
type Announcer interface {
	Announce(n Notice, cycle int)
}

// Store persists the cycle number and the nobles' records.
type Store interface {
	// LoadCycle returns the stored cycle number, found=false when none was
	// stored yet.
	LoadCycle(ctx context.Context) (cycle int, found bool, err error)
	SaveCycle(ctx context.Context, cycle int) error
	// LoadNobles returns the record of every noble whose character still
	// exists, keyed by character object id.
	LoadNobles(ctx context.Context) (map[int32]Noble, error)
	// SaveNobles inserts or updates each record.
	SaveNobles(ctx context.Context, nobles map[int32]Noble) error
	// DeleteNobles removes every record.
	DeleteNobles(ctx context.Context) error
	// SnapshotMonth replaces the month's standings with a copy of the
	// stored records.
	SnapshotMonth(ctx context.Context) error
}

// step is one scheduled calendar change. The order breaks ties between
// steps due at the same moment: the earlier one runs first.
type step int

const (
	stepNoblePoints step = iota
	stepValidationEnd
	stepCompetitionEnd
	stepOlympiadEnd
	stepRegistrationEnd
	stepCount
)

// Olympiad is the calendar and the nobles' records. Its calendar steps run
// on its queue; the reads are safe from any goroutine.
type Olympiad struct {
	cfg   Config
	store Store
	out   Announcer
	log   zerolog.Logger
	queue *sim.Queue

	// run serializes the calendar steps with Stop's save, and is held
	// across their database writes. It guards the fields below it.
	run     sync.Mutex
	stopped bool
	timer   *sim.Timer
	due     step
	// olympiadEnd and nextPoints are Unix milliseconds.
	olympiadEnd int64
	nextPoints  int64
	// registrationEnd is how long, from the latest scheduling, registration
	// stays open; negative when it is not counting down.
	registrationEnd int64

	// mu guards the fields below it; never held across I/O or an
	// announcement.
	mu     sync.Mutex
	cycle  int
	period Period
	// periodEnd is in Unix milliseconds.
	periodEnd int64
	nobles    map[int32]Noble
}

// New returns an Olympiad persisting through store and announcing through
// out. Its calendar runs on queue, which it owns from then on. Restore then
// Start bring it up.
func New(cfg Config, store Store, out Announcer, queue *sim.Queue, log zerolog.Logger) *Olympiad {
	return &Olympiad{cfg: cfg, store: store, out: out, queue: queue, log: log, registrationEnd: -1, nobles: map[int32]Noble{}}
}

// Restore loads the cycle number, the first when none is stored, and the
// nobles' records.
func (o *Olympiad) Restore(ctx context.Context) error {
	cycle, found, err := o.store.LoadCycle(ctx)
	if err != nil {
		return err
	}
	if !found {
		cycle = 1
	}
	nobles, err := o.store.LoadNobles(ctx)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.cycle = cycle
	o.nobles = nobles
	o.mu.Unlock()
	o.log.Info().Int("nobles", len(nobles)).Int("cycle", cycle).Msg("olympiad: restored")
	return nil
}

// Start sets the Olympiad's end a month ahead, enters the period the clock
// is in and schedules the next calendar step.
func (o *Olympiad) Start() {
	o.queue.Post(func() {
		o.run.Lock()
		defer o.run.Unlock()
		if o.stopped {
			return
		}
		o.olympiadEnd = nextOlympiadEnd(o.queue.Now())
		o.enterPeriod()
		o.schedule()
	})
}

// Stop cancels the pending calendar step, waits for a running one, and
// saves the cycle number and the nobles' records within TaskTimeout.
func (o *Olympiad) Stop(ctx context.Context) {
	o.queue.Close()
	o.run.Lock()
	defer o.run.Unlock()
	o.stopped = true
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
	ctx, cancel := context.WithTimeout(ctx, TaskTimeout)
	defer cancel()
	o.saveStatus(ctx)
}

// Noble returns objectID's record for the running cycle.
func (o *Olympiad) Noble(objectID int32) (Noble, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, ok := o.nobles[objectID]
	return n, ok
}

// Period returns the period the Olympiad is in.
func (o *Olympiad) Period() Period {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.period
}

// Cycle returns the running cycle's number.
func (o *Olympiad) Cycle() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cycle
}

// fire runs the due calendar step, on the queue, then schedules the next.
func (o *Olympiad) fire() {
	o.run.Lock()
	defer o.run.Unlock()
	if o.stopped {
		return
	}
	o.timer = nil
	ctx, cancel := context.WithTimeout(context.Background(), TaskTimeout)
	defer cancel()
	switch o.due {
	case stepCompetitionEnd:
		o.out.Announce(NoticeCompetitionEnded, 0)
		o.closeCompetition(ctx)
	case stepValidationEnd:
		o.startCycle()
		o.deleteNobles(ctx)
		o.enterPeriod()
	case stepNoblePoints:
		o.nextPoints = nextNoblePointsUpdate(o.queue.Now(), o.cfg)
		o.mu.Lock()
		for id, n := range o.nobles {
			n.addPoints(o.cfg.WeeklyPoints)
			o.nobles[id] = n
		}
		o.mu.Unlock()
	case stepRegistrationEnd:
		o.out.Announce(NoticeRegistrationEnded, 0)
		o.registrationEnd = -1
	case stepOlympiadEnd:
		o.endOlympiad(ctx)
	}
	o.schedule()
}

// schedule arms the timer for the step due soonest; ties go to the step
// listed first. The competition window's end is a candidate only during the
// competition, the validation period's only outside it.
func (o *Olympiad) schedule() {
	now := o.queue.Now().UnixMilli()
	o.mu.Lock()
	period, periodEnd := o.period, o.periodEnd
	o.mu.Unlock()

	best, bestDelay := stepCount, int64(math.MaxInt64)
	for s := range stepCount {
		var delay int64
		switch s {
		case stepCompetitionEnd, stepValidationEnd:
			if (s == stepCompetitionEnd) != (period == Competition) {
				continue
			}
			delay = periodEnd - now
		case stepOlympiadEnd:
			delay = o.olympiadEnd - now
		case stepNoblePoints:
			delay = o.nextPoints - now
		case stepRegistrationEnd:
			delay = o.registrationEnd
		}
		if delay >= 0 && delay < bestDelay {
			best, bestDelay = s, delay
		}
	}
	if best == stepCount {
		o.log.Warn().Msg("olympiad: no calendar step to schedule")
		return
	}
	o.due = best
	o.timer = o.queue.After(time.Duration(bestDelay)*time.Millisecond, o.fire)
}

// enterPeriod renews a reached Olympiad end and a passed weekly grant, then
// enters the period the clock is in. Entering a competition window opens
// it; a validation period found already over opens a new cycle and the
// window.
//
// The end is renewed once reached, not only once passed: a step due at the
// end itself would otherwise find it still ahead, enter a period clamped to
// end now, and fire again at once, for as long as the clock reads the same
// millisecond.
func (o *Olympiad) enterPeriod() {
	now := o.queue.Now()
	nowMs := now.UnixMilli()
	if o.olympiadEnd == 0 || o.olympiadEnd <= nowMs {
		o.olympiadEnd = nextOlympiadEnd(now)
	}
	if o.nextPoints == 0 || o.nextPoints < nowMs {
		o.nextPoints = nextNoblePointsUpdate(now, o.cfg)
	}
	period, end := periodAt(now, o.cfg, o.olympiadEnd)
	o.mu.Lock()
	o.period, o.periodEnd = period, end
	o.mu.Unlock()

	if period == Competition {
		o.openCompetition()
		return
	}
	o.registrationEnd = -1
	if end <= o.queue.Now().UnixMilli() {
		o.startCycle()
		o.openCompetition()
	}
}

// openCompetition announces the competition window and starts the
// countdown to its registration's close, ten minutes before it ends. The
// matches are not run yet (#217).
func (o *Olympiad) openCompetition() {
	o.out.Announce(NoticeCompetitionStarted, 0)
	o.mu.Lock()
	end := o.periodEnd
	o.mu.Unlock()
	o.registrationEnd = end - o.queue.Now().UnixMilli() - registrationCloseMillis
}

// closeCompetition saves the status once the window's matches are over and
// enters the next period. No match can still be running: matches are not
// run yet (#217).
func (o *Olympiad) closeCompetition(ctx context.Context) {
	o.saveStatus(ctx)
	o.enterPeriod()
}

// startCycle opens the next cycle in the competition period.
func (o *Olympiad) startCycle() {
	o.mu.Lock()
	o.period = Competition
	o.cycle++
	cycle := o.cycle
	o.mu.Unlock()
	o.out.Announce(NoticeCycleStarted, cycle)
}

// endOlympiad closes the running Olympiad: it saves the records, enters the
// validation period, saves the status, keeps the month's standings and
// enters the period the clock is in. No heroes are elected yet (#220).
func (o *Olympiad) endOlympiad(ctx context.Context) {
	o.out.Announce(NoticeCycleEnded, o.Cycle())
	o.saveNobles(ctx)
	o.mu.Lock()
	o.period = Validation
	o.mu.Unlock()
	o.saveStatus(ctx)
	if err := o.store.SnapshotMonth(ctx); err != nil {
		o.log.Error().Err(err).Msg("olympiad: keep the month's standings")
	}
	o.enterPeriod()
}

// saveStatus saves the cycle number and the nobles' records.
func (o *Olympiad) saveStatus(ctx context.Context) {
	if err := o.store.SaveCycle(ctx, o.Cycle()); err != nil {
		o.log.Error().Err(err).Msg("olympiad: save cycle")
	}
	o.saveNobles(ctx)
}

// saveNobles saves every noble's record; none held writes nothing.
func (o *Olympiad) saveNobles(ctx context.Context) {
	o.mu.Lock()
	nobles := maps.Clone(o.nobles)
	o.mu.Unlock()
	if len(nobles) == 0 {
		return
	}
	if err := o.store.SaveNobles(ctx, nobles); err != nil {
		o.log.Error().Err(err).Msg("olympiad: save nobles")
	}
}

// deleteNobles removes every noble's record, stored and held; the held ones
// go even when the stored ones could not be removed.
func (o *Olympiad) deleteNobles(ctx context.Context) {
	if err := o.store.DeleteNobles(ctx); err != nil {
		o.log.Error().Err(err).Msg("olympiad: delete nobles")
	}
	o.mu.Lock()
	clear(o.nobles)
	o.mu.Unlock()
}
