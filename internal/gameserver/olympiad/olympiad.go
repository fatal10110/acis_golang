// Package olympiad owns the Olympiad calendar and the nobles' records.
//
// An Olympiad is one cycle: it runs until noon on the first day of the next
// month. Inside it each day has a competition window, from the configured
// start time for the configured length, and a weekly grant adds points to
// every noble's record. Its end elects the heroes and keeps a copy of the
// records as the month's standings; a validation period of a day follows,
// with no competition window, in which nobles trade their points for
// Noblesse Gate Passes. Only once the validation period is over does the
// next cycle open, with the records cleared. The cycle number, the
// records and a running validation period persist across restarts.
//
// The waiting lists nobles register on during a competition window, and
// the trade of a cycle's points for Noblesse Gate Passes, are in
// registration.go. The rules of a match between two nobles are in game.go;
// the stadiums that run the matches, in stadium.go, are filled by the draws
// of games.go while a competition window is open. The end of an Olympiad
// elects the heroes, behind the save of the records they are elected from.
package olympiad

import (
	"context"
	"errors"
	"maps"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// TaskTimeout bounds one database write.
const TaskTimeout = 10 * time.Second

// writeLane is the persistence owner every Olympiad write is queued under:
// one lane keeps the writes in the order the calendar made them, so a
// truncate never lands after the records saved behind it. No character,
// item or clan has object id 0.
const writeLane int32 = 0

// Period is the part of the calendar the Olympiad is in.
type Period int

const (
	// Competition is a daily match window of a running Olympiad.
	Competition Period = iota
	// Validation follows the end of an Olympiad until the next cycle opens:
	// no window opens, and nobles trade their points for passes.
	Validation
	// Interval is the time between two competition windows of a running
	// Olympiad.
	Interval
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
	// MaxPoints caps the points one match moves.
	MaxPoints int
	// DividerClassed and DividerNonClassed divide the lower of the two
	// competitors' points into what a classed or non-classed match moves.
	DividerClassed, DividerNonClassed int
	// ClassedReward and NonClassedReward are the items the winner of a
	// classed or non-classed match is given.
	ClassedReward, NonClassedReward []Reward
	// StartPoints are the points a noble's record starts a cycle with.
	StartPoints int
	// MinMatches is how many matches a noble needs in a month to stand in
	// its class's ranking.
	MinMatches int
	// GPPerPoint is how many Noblesse Gate Passes one point trades for, and
	// HeroPoints the points a hero trades on top of its own.
	GPPerPoint, HeroPoints int
	// BattleMillis is how long a match's fight may last.
	BattleMillis int64
	// WaitTime, WaitBattle and WaitEnd are the seconds counted down before
	// a match's competitors enter the stadium, before they may fight, and
	// before they are sent back.
	WaitTime, WaitBattle, WaitEnd int
	// ClassedParticipants and NonClassedParticipants are how many
	// registrations a class queue and the non-classed list need for a draw
	// to take matches from them.
	ClassedParticipants, NonClassedParticipants int
	// AnnounceGames has the Grand Olympiad Managers shout each match as its
	// competitors enter the stadium.
	AnnounceGames bool
}

// Reward is a stack of items a match's winner is given.
type Reward struct {
	ItemID int32
	Count  int
}

// DefaultConfig returns the configuration used when no setting overrides
// it: a six-hour window opening at 18:00, three weekly points; matches
// moving at most 10 points, a third of the lower total in classed matches
// and a fifth in non-classed ones, their winners given 50 and 30 Noblesse
// Gate Passes; records starting with 18 points, rankings counting nobles
// with 5 matches, and 1000 passes a point with 300 more points for a hero;
// fights of three minutes, after 30 seconds to enter the stadium and 60 to
// start, then 40 to be sent back; draws needing 5 registrations of a class
// or 9 non-classed ones, and the managers shouting each match.
func DefaultConfig() Config {
	return Config{
		StartHour: 18, CompetitionMillis: 21600000, WeeklyPoints: 3,
		MaxPoints: 10, DividerClassed: 3, DividerNonClassed: 5,
		ClassedReward:    []Reward{{ItemID: 6651, Count: 50}},
		NonClassedReward: []Reward{{ItemID: 6651, Count: 30}},
		StartPoints:      18, MinMatches: 5, GPPerPoint: 1000, HeroPoints: 300,
		BattleMillis: 180000, WaitTime: 30, WaitBattle: 60, WaitEnd: 40,
		ClassedParticipants: 5, NonClassedParticipants: 9, AnnounceGames: true,
	}
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
	Announce(n Notice, cycle int32)
}

// Writer runs a database write later, on ownerID's persistence lane, so
// the calendar never waits on the database. Flush waits for the writes
// queued on ownerIDs' lanes.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
	Flush(ctx context.Context, ownerIDs ...int32) error
}

// Status is the calendar state kept across restarts.
type Status struct {
	Cycle int32
	// ValidationEnd is when the validation period running after an
	// Olympiad's end closes, in Unix milliseconds; zero outside one.
	ValidationEnd int64
}

// Store persists the calendar's status and the nobles' records.
type Store interface {
	// LoadStatus returns the stored status, found=false when no cycle was
	// stored yet.
	LoadStatus(ctx context.Context) (st Status, found bool, err error)
	SaveStatus(ctx context.Context, st Status) error
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
	// SaveFight stores the result of one match.
	SaveFight(ctx context.Context, f Fight) error
	// ClassLeaders returns the names of the ten best of classID in the
	// month's standings with at least minMatches matches, by points, then
	// matches, then wins.
	ClassLeaders(ctx context.Context, classID, minMatches int) ([]string, error)
}

// Heroes elects the heroes from the stored nobles' records.
type Heroes interface {
	// Elect runs on the Olympiad's persistence lane, so it reads the
	// records the Olympiad saved before it.
	Elect(ctx context.Context)
}

// step is one scheduled calendar change. The order breaks ties between
// steps due at the same moment: the earlier one runs first, so a window
// cut short by the Olympiad's end closes before the end runs, and the end
// runs before a window due to open at that moment.
type step int

const (
	stepNoblePoints step = iota
	stepCompetitionEnd
	stepOlympiadEnd
	stepCompetitionStart
	stepValidationEnd
	stepRegistrationEnd
	stepCount
)

// periodEnding is the period each of the steps ending one closes.
var periodEnding = [stepCount]Period{stepCompetitionEnd: Competition, stepCompetitionStart: Interval, stepValidationEnd: Validation}

// Olympiad is the calendar and the nobles' records. Its calendar steps run
// on its queue and hand their database writes to its writer; the reads are
// safe from any goroutine.
type Olympiad struct {
	cfg    Config
	store  Store
	writes Writer
	out    Announcer
	heroes Heroes
	log    zerolog.Logger
	queue  *sim.Queue

	// run serializes the calendar steps with Stop. It guards the fields
	// below it.
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

	// games is the game manager BindStadiums sets before Start; nil runs
	// no match.
	games *games

	// order keeps the records' writes in the order the records changed: it
	// is held from a change to the records, or the copy of them a write
	// takes, until that write is queued.
	order sync.Mutex

	// mu guards the fields below it; never held across I/O or an
	// announcement.
	mu     sync.Mutex
	cycle  int32
	period Period
	// periodEnd is when period ends, in Unix milliseconds: the window's
	// close, the next window's opening, or the validation period's close.
	periodEnd int64
	nobles    map[int32]Noble
	// classed holds the waiting list of each base class anyone registered
	// for, nonClassed the one for non-classed matches, both in registration
	// order.
	classed    map[int][]int32
	nonClassed []int32
}

// New returns an Olympiad persisting through store, its writes queued on
// writes (run inline when nil), announcing through out, and electing the
// heroes at its end through heroes (none elected when nil). Its calendar
// runs on queue, which it owns from then on. Restore then Start bring it
// up.
func New(cfg Config, store Store, writes Writer, out Announcer, heroes Heroes, queue *sim.Queue, log zerolog.Logger) *Olympiad {
	return &Olympiad{cfg: cfg, store: store, writes: writes, out: out, heroes: heroes, queue: queue, log: log, registrationEnd: -1, nobles: map[int32]Noble{}, classed: map[int][]int32{}}
}

// Restore loads the cycle number, the first when none is stored, a running
// validation period and the nobles' records.
func (o *Olympiad) Restore(ctx context.Context) error {
	st, found, err := o.store.LoadStatus(ctx)
	if err != nil {
		return err
	}
	if !found {
		st.Cycle = 1
	}
	nobles, err := o.store.LoadNobles(ctx)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.cycle = st.Cycle
	if st.ValidationEnd != 0 {
		o.period, o.periodEnd = Validation, st.ValidationEnd
	}
	o.nobles = nobles
	o.mu.Unlock()
	o.log.Info().Int("nobles", len(nobles)).Int32("cycle", st.Cycle).Msg("olympiad: restored")
	return nil
}

// Start schedules the next calendar step. A restored validation period
// runs to its end, or opens the next cycle at once when already over;
// otherwise the Olympiad ends at noon on the first of the next month, and
// the period the clock is in is entered.
func (o *Olympiad) Start() {
	o.queue.Post(func() {
		o.run.Lock()
		defer o.run.Unlock()
		if o.stopped {
			return
		}
		now := o.queue.Now()
		o.mu.Lock()
		validation, validationEnd := o.period == Validation, o.periodEnd
		o.mu.Unlock()
		switch {
		case !validation:
			o.olympiadEnd = nextOlympiadEnd(now)
			o.enterPeriod()
		case validationEnd <= now.UnixMilli():
			o.startCycle()
		}
		o.schedule()
	})
}

// Stop cancels the pending calendar step, waits for a running one, queues
// the save of the cycle number and the nobles' records, and waits until
// ctx ends for every write it queued to land. A write still queued then
// lands when the writer drains.
func (o *Olympiad) Stop(ctx context.Context) {
	o.queue.Close()
	o.run.Lock()
	defer o.run.Unlock()
	o.stopped = true
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
	o.stopGames()
	if o.games != nil {
		o.games.close()
	}
	o.saveStatus()
	if o.writes == nil {
		return
	}
	if err := o.writes.Flush(ctx, writeLane); err != nil {
		o.log.Error().Err(err).Msg("olympiad: wait for the final save")
	}
}

// Noble returns objectID's record for the running cycle.
func (o *Olympiad) Noble(objectID int32) (Noble, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, ok := o.nobles[objectID]
	return n, ok
}

// updateNoble changes objectID's record in place; a noble without one is
// left without.
func (o *Olympiad) updateNoble(objectID int32, change func(*Noble)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if n, ok := o.nobles[objectID]; ok {
		change(&n)
		o.nobles[objectID] = n
	}
}

// Period returns the period the Olympiad is in.
func (o *Olympiad) Period() Period {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.period
}

// Cycle returns the running cycle's number.
func (o *Olympiad) Cycle() int32 {
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
	switch o.due {
	case stepCompetitionEnd:
		o.out.Announce(NoticeCompetitionEnded, 0)
		o.closeCompetition()
	case stepCompetitionStart:
		o.enterPeriod()
	case stepValidationEnd:
		o.startCycle()
	case stepNoblePoints:
		// A grant due in the validation period adds nothing.
		o.nextPoints = nextNoblePointsUpdate(o.queue.Now(), o.cfg)
		o.mu.Lock()
		if o.period != Validation {
			for id, n := range o.nobles {
				n.addPoints(o.cfg.WeeklyPoints)
				o.nobles[id] = n
			}
		}
		o.mu.Unlock()
	case stepRegistrationEnd:
		o.out.Announce(NoticeRegistrationEnded, 0)
		o.registrationEnd = -1
	case stepOlympiadEnd:
		o.endOlympiad()
	}
	o.schedule()
}

// schedule arms the timer for the step due soonest; ties go to the step
// listed first. Each period's own end is a candidate only during it, the
// Olympiad's end only outside the validation period, and the weekly grant
// once it is set. A step the clock has passed is due at once.
func (o *Olympiad) schedule() {
	now := o.queue.Now().UnixMilli()
	o.mu.Lock()
	period, periodEnd := o.period, o.periodEnd
	o.mu.Unlock()

	best, bestDelay := stepCount, int64(math.MaxInt64)
	for s := range stepCount {
		var delay int64
		switch s {
		case stepCompetitionEnd, stepCompetitionStart, stepValidationEnd:
			if period != periodEnding[s] {
				continue
			}
			delay = max(periodEnd-now, 0)
		case stepOlympiadEnd:
			if period == Validation {
				continue
			}
			delay = max(o.olympiadEnd-now, 0)
		case stepNoblePoints:
			if o.nextPoints == 0 {
				continue
			}
			delay = max(o.nextPoints-now, 0)
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

// enterPeriod sets a weekly grant not set or passed, then enters the part
// of the running Olympiad's day the clock is in, opening the competition
// window when inside it.
func (o *Olympiad) enterPeriod() {
	now := o.queue.Now()
	if o.nextPoints == 0 || o.nextPoints < now.UnixMilli() {
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
	o.stopGames()
}

// openCompetition announces the competition window, starts its draws and
// the countdown to its registration's close, ten minutes before it ends.
func (o *Olympiad) openCompetition() {
	o.out.Announce(NoticeCompetitionStarted, 0)
	o.startGames()
	o.mu.Lock()
	end := o.periodEnd
	o.mu.Unlock()
	o.registrationEnd = end - o.queue.Now().UnixMilli() - registrationCloseMillis
}

// closeCompetition stops the window's draws, saves the status and enters
// the next period. A match still running goes on to its end in its stadium
// and is saved with the next status save.
func (o *Olympiad) closeCompetition() {
	o.stopGames()
	o.saveStatus()
	o.enterPeriod()
}

// startCycle closes the validation period: the next cycle opens with no
// records, its Olympiad ending at noon on the first of the next month; the
// status is saved, so a restart opens no second cycle, and the period the
// clock is in is entered.
func (o *Olympiad) startCycle() {
	o.mu.Lock()
	o.period = Interval
	o.cycle++
	cycle := o.cycle
	o.mu.Unlock()
	o.deleteNobles()
	o.out.Announce(NoticeCycleStarted, cycle)
	o.olympiadEnd = nextOlympiadEnd(o.queue.Now())
	o.saveStatus()
	o.enterPeriod()
}

// SelectHeroes ends the running Olympiad at once, as its end would: the
// pending calendar step is cancelled, an open competition window closes
// with no announcement, the heroes are elected from the records as they
// stand and the validation period starts, the next cycle opening once it
// is over. Run during a validation period it ends the Olympiad again, the
// validation period starting over.
func (o *Olympiad) SelectHeroes() {
	o.queue.Post(func() {
		o.run.Lock()
		defer o.run.Unlock()
		if o.stopped {
			return
		}
		if o.timer != nil {
			o.timer.Stop()
			o.timer = nil
		}
		o.endOlympiad()
		o.schedule()
	})
}

// endOlympiad closes the running Olympiad: it stops the draws, saves the
// records, enters the validation period, elects the heroes from the saved
// records, saves the status and keeps the month's standings.
func (o *Olympiad) endOlympiad() {
	o.stopGames()
	o.registrationEnd = -1
	o.out.Announce(NoticeCycleEnded, o.Cycle())
	o.saveNobles()
	o.mu.Lock()
	o.period, o.periodEnd = Validation, o.queue.Now().UnixMilli()+validationMillis
	o.mu.Unlock()
	if o.heroes != nil {
		heroes := o.heroes
		o.queueJob("elect heroes", func(ctx context.Context) error {
			heroes.Elect(ctx)
			return nil
		})
	}
	o.saveStatus()
	o.write("keep the month's standings", func(ctx context.Context, st Store) error { return st.SnapshotMonth(ctx) })
}

// saveStatus queues the save of the status and the nobles' records.
func (o *Olympiad) saveStatus() {
	o.mu.Lock()
	st := Status{Cycle: o.cycle}
	if o.period == Validation {
		st.ValidationEnd = o.periodEnd
	}
	o.mu.Unlock()
	o.write("save status", func(ctx context.Context, s Store) error { return s.SaveStatus(ctx, st) })
	o.saveNobles()
}

// saveNobles queues the save of every noble's record as it stands now;
// none held writes nothing.
func (o *Olympiad) saveNobles() {
	o.order.Lock()
	defer o.order.Unlock()
	o.mu.Lock()
	nobles := maps.Clone(o.nobles)
	o.mu.Unlock()
	if len(nobles) == 0 {
		return
	}
	o.write("save nobles", func(ctx context.Context, st Store) error { return st.SaveNobles(ctx, nobles) })
}

// deleteNobles removes every noble's record held and queues the removal of
// the stored ones; the held ones go even when the stored ones cannot.
func (o *Olympiad) deleteNobles() {
	o.order.Lock()
	defer o.order.Unlock()
	o.write("delete nobles", func(ctx context.Context, st Store) error { return st.DeleteNobles(ctx) })
	o.mu.Lock()
	clear(o.nobles)
	o.mu.Unlock()
}

// write queues fn on the Olympiad's persistence lane, or runs it at once
// without a writer. Each write gets TaskTimeout.
func (o *Olympiad) write(what string, fn func(context.Context, Store) error) {
	store := o.store
	o.queueJob(what, func(ctx context.Context) error { return fn(ctx, store) })
}

// writeLanded is write, returning a channel that receives fn's result once
// it ran, or errWriteDropped when it never will.
func (o *Olympiad) writeLanded(what string, fn func(context.Context, Store) error) <-chan error {
	store := o.store
	landed := make(chan error, 1)
	if !o.queueJob(what, func(ctx context.Context) error {
		err := fn(ctx, store)
		landed <- err
		return err
	}) {
		landed <- errWriteDropped
	}
	return landed
}

// errWriteDropped is the result of a write the writer refused.
var errWriteDropped = errors.New("olympiad: write dropped")

// awaitLanded waits until ctx ends for the result writeLanded sends on
// landed; one already sent wins over an ended ctx.
func awaitLanded(ctx context.Context, landed <-chan error) error {
	select {
	case err := <-landed:
		return err
	default:
	}
	select {
	case err := <-landed:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// queueJob queues fn on the Olympiad's persistence lane, or runs it at once
// without a writer, reporting false when the writer refused it. Each job
// gets TaskTimeout.
func (o *Olympiad) queueJob(what string, fn func(context.Context) error) bool {
	log := o.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), TaskTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			log.Error().Err(err).Msg("olympiad: " + what)
		}
	}
	if o.writes == nil {
		job()
		return true
	}
	if !o.writes.Enqueue(writeLane, job) {
		log.Error().Msg("olympiad: " + what + ": write dropped")
		return false
	}
	return true
}
