package script

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// Server is the server-wide state the scheduled tasks act on. Each method
// handles and logs its own failures.
type Server interface {
	// UpdateCastleTaxes closes every castle's tax period.
	UpdateCastleTaxes()
	// SealValidationPeriod reports whether the Seven Signs are in their
	// seal validation period.
	SealValidationPeriod() bool
	// SaveFestivalScores stores the festival's scores.
	SaveFestivalScores(ctx context.Context)
	// SaveSevenSigns stores the Seven Signs sign-ups and status.
	SaveSevenSigns(ctx context.Context)
	// TransferClanLeaders hands every clan with a pending leader
	// nomination to its nominee.
	TransferClanLeaders()
	// RefreshClanLadder ranks the clans again by reputation.
	RefreshClanLadder()
	// RefreshRecommendations gives every player, online and stored, its
	// daily recommendations.
	RefreshRecommendations(ctx context.Context)
	// RaidPointWinners returns the object ids of the first 100 players of
	// the raid point ranking, first place first.
	RaidPointWinners() []int32
	// MemberClan returns the id and level of the clan objectID is a
	// member of; ok is false when it is in none.
	MemberClan(objectID int32) (clanID int32, level int, ok bool)
	// AddClanReputation adds points to the reputation of clan clanID and
	// shows its members the new score.
	AddClanReputation(clanID int32, points int)
	// CleanUpRaidPoints forgets and clears every player's raid points.
	CleanUpRaidPoints()
}

// rescanPeriod is how often the runner looks for the tasks whose next
// start falls within the coming period, and the length of that period.
const rescanPeriod = 5 * time.Minute

// dueWithin reports whether a start at next falls before now plus
// rescanPeriod, and if so the delay until it, negative when it is past: a
// start exactly one period away waits for the next look.
func dueWithin(now, next time.Time) (time.Duration, bool) {
	nowMs, nextMs := now.UnixMilli(), next.UnixMilli()
	if nowMs+rescanPeriod.Milliseconds()-nextMs <= 0 {
		return 0, false
	}
	return time.Duration(nextMs-nowMs) * time.Millisecond, true
}

// Schedule runs the registry's scheduled tasks: every listed script with a
// start hook whose entry names a schedule. All of its state belongs to its
// queue.
type Schedule struct {
	r      *Registry
	queue  *sim.Queue
	srv    Server
	ctx    context.Context
	cancel context.CancelFunc
	tasks  []*scheduledTask
}

// scheduledTask is one task's next start and the timer armed for it.
type scheduledTask struct {
	s     *Script
	sc    schedule
	next  time.Time
	timer *sim.Timer
}

// StartSchedule runs r's scheduled tasks on queue, which it owns from then
// on, against srv, on the queue's clock and in its time zone. Each task's
// first start is set from now; then, now and every rescanPeriod, each task
// whose next start falls within the coming period is armed for it. A
// firing moves the task's next start on by one period, runs its start hook
// and arms the task again when that next start is also that close.
func StartSchedule(r *Registry, queue *sim.Queue, srv Server) *Schedule {
	ctx, cancel := context.WithCancel(context.Background())
	sch := &Schedule{r: r, queue: queue, srv: srv, ctx: ctx, cancel: cancel}
	queue.Post(func() {
		now := queue.Now()
		for _, e := range r.entries {
			if e.sched != nil {
				sch.tasks = append(sch.tasks, &scheduledTask{s: e.script, sc: *e.sched, next: e.sched.first(now, usWeek)})
			}
		}
		sch.rescan()
		queue.Every(rescanPeriod, sch.rescan)
	})
	return sch
}

// Stop stops every timer and cancels the context of a start hook still
// running.
func (sch *Schedule) Stop() {
	sch.queue.Close()
	sch.cancel()
}

// rescan arms each task whose next start falls within the coming period.
func (sch *Schedule) rescan() {
	now := sch.queue.Now()
	for _, t := range sch.tasks {
		if d, ok := dueWithin(now, t.next); ok {
			sch.arm(t, d)
		}
	}
}

// arm fires t after d, replacing a firing already armed.
func (sch *Schedule) arm(t *scheduledTask, d time.Duration) {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = sch.queue.After(max(d, 0), func() { sch.fire(t) })
}

// fire runs one start of t.
func (sch *Schedule) fire(t *scheduledTask) {
	t.timer = nil
	t.next = t.sc.next(t.next)
	e := Start{Ctx: sch.ctx, Server: sch.srv}
	sch.r.run(t.s, hookStart, func() { t.s.Hooks.Start(t.s, e) })
	if d, ok := dueWithin(sch.queue.Now(), t.next); ok {
		sch.arm(t, d)
	}
}
