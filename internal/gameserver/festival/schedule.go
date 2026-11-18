package festival

import (
	"context"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// schedule is the festival schedule's state, guarded by Manager.mu.
//
// A started schedule waits ManagerStart, then runs its first festival
// cycle, retrying a whole cycle length later while seal validation is under
// way or the Seven Signs period ends before a cycle could finish. A running
// cycle waits for the signup period to end; with no party signed up it
// opens the next signup period at once, for as long as nobody signs up,
// whatever the Seven Signs period.
type schedule struct {
	// nextFestivalStart is in Unix milliseconds; zero until a schedule
	// first sets it.
	nextFestivalStart int64
	// nextCycleStart is when the next festival cycle is due, in Unix
	// milliseconds: a cycle length after the schedule started or the last
	// signup period opened; zero until a schedule first sets it.
	nextCycleStart int64
	timer          Timer
	// generation tells a timer of a cancelled schedule from the running
	// one's: each start and cancel moves it on.
	generation uint64
	// running is set while a cycle is under way: from its start, past
	// seal validation and the period end checks, until its festival ends.
	running bool
	// stopped latches Stop: nothing starts a schedule afterwards.
	stopped bool
}

// Start starts the festival schedule unless seal validation is under way;
// the Seven Signs starts it again when the next competition begins.
// Restore must have completed first.
func (m *Manager) Start() {
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation {
		m.log.Info().Msg("festival: not started during seal validation")
		return
	}
	m.CompetitionBegun()
}

// Stop cancels the schedule for good, then waits, until ctx ends, for the
// steps still acting on the world; once it returns before ctx ends the
// festival's timers neither post to a queue nor write. It must not run on
// the festival's queue.
func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	m.cancelLocked()
	if m.current != nil {
		if m.current.timer != nil {
			m.current.timer.Stop()
		}
		m.current = nil
	}
	m.waitIdleLocked(ctx)
}

// WaitIdle returns once no timer step is acting on the world. It must not
// run on the festival's queue.
func (m *Manager) WaitIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.waitIdleLocked(context.Background())
}

// waitIdleLocked waits on m.mu until no step is acting on the world or ctx
// ends.
func (m *Manager) waitIdleLocked(ctx context.Context) {
	if m.steps == 0 {
		return
	}
	stop := context.AfterFunc(ctx, func() {
		m.mu.Lock()
		m.idle.Broadcast()
		m.mu.Unlock()
	})
	defer stop()
	for m.steps > 0 && ctx.Err() == nil {
		m.idle.Wait()
	}
}

// CompetitionBegun starts a new festival schedule in place of any running
// one and counts a new festival cycle. A festival under way runs to its
// end alongside it.
func (m *Manager) CompetitionBegun() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.cancelLocked()
	m.running, m.lastRun = false, false
	m.started = true
	m.status.FestivalCycle++
	now := m.nowMillis()
	start := m.cfg.ManagerStart.Milliseconds()
	m.nextFestivalStart = now + start + m.cfg.signup().Milliseconds()
	m.nextCycleStart = now + m.cfg.CycleLength.Milliseconds()
	generation, due := m.generation, now+start
	m.armLocked(start, func() { m.runCycle(generation, due) })
	m.log.Info().Int64("minutes", start/60000).Msg("festival: the first festival cycle begins")
}

// CompetitionEnded stops the festival schedule, then rewards the clans of
// the party that set each festival's best score ever: for each festival in
// turn, each member named, online or not, earns its clan 100 reputation
// points, once per festival it holds the record of. then runs once every
// reward has been told.
//
// Only a cycle not yet begun is called off. A cycle under way is the last:
// while nobody signs up it keeps opening a new signup period whenever one
// ends, through the results, seal validation and recruiting periods, so
// NextFestivalNotice keeps counting a fresh 1 to 20 minutes down. The
// festival a party signs up for runs to its end, its parties taken back
// out of their arenas, and no cycle follows it. CompetitionBegun replaces
// the cycle still opening signup periods with a new schedule.
func (m *Manager) CompetitionEnded(then func()) {
	m.mu.Lock()
	if m.running {
		m.lastRun = true
	} else {
		m.cancelLocked()
	}
	var names []string
	for id := range Count {
		if best, ok := m.overallHighestLocked(id); ok {
			names = append(names, best.MemberNames()...)
		}
	}
	world := m.world
	m.mu.Unlock()
	if world == nil {
		then()
		return
	}
	m.rewardClans(world, names, then)
}

// rewardClans rewards the clan of each of names in turn, each once the one
// before has been told, then runs then.
func (m *Manager) rewardClans(world World, names []string, then func()) {
	if len(names) == 0 {
		then()
		return
	}
	world.RewardClanOf(names[0], highestRankedReward, m.onQueue(func() { m.rewardClans(world, names[1:], then) }))
}

// runCycle runs the festival cycle that was due at due. During seal
// validation, or when the Seven Signs period ends before a whole cycle
// could run, it skips this cycle and waits for the next one, a cycle
// length after due. Otherwise, when the next festival is two minutes off,
// the guides call for the parties to sign up, and the signup period runs.
func (m *Manager) runCycle(generation uint64, due int64) {
	m.step(generation, func() func(World) {
		now := m.nowMillis()
		cycle := m.cfg.CycleLength.Milliseconds()
		if m.calendar.CurrentPeriod() == sevensigns.SealValidation || m.calendar.NextChange().UnixMilli()-now < cycle {
			next := due + cycle
			m.armLocked(max(0, next-now), func() { m.runCycle(generation, next) })
			return nil
		}
		m.running = true
		shout := m.minutesToNextFestivalLocked() == 2
		m.armLocked(m.cfg.signup().Milliseconds(), func() { m.signupEnded(generation, due) })
		if !shout {
			return nil
		}
		return func(world World) { world.Shout(guideName, twoMinutesShout) }
	})
}

// signupEnded ends a signup period: the parties of the last festival may
// no longer register their scores. With no festival taken, the next signup
// period opens at once and ends a festival's length and a minute later.
// Otherwise the festival begins: each party is taken into its arena and
// the guides announce it.
//
// A festival the schedule this one replaced began, still under way, holds
// its parties until it ends; until then each signup period ends as if
// nobody had signed up. The shipped timings end it before the replacing
// schedule's first signup does.
func (m *Manager) signupEnded(generation uint64, due int64) {
	m.step(generation, func() func(World) {
		m.previous = [2]roster{{}, {}}
		if m.current != nil || len(m.participants[0]) == 0 && len(m.participants[1]) == 0 {
			now := m.nowMillis()
			wait := (m.cfg.CycleLength - m.cfg.signup()).Milliseconds()
			m.nextFestivalStart = now + wait
			m.nextCycleStart = now + m.cfg.CycleLength.Milliseconds()
			m.armLocked(wait, func() { m.signupEnded(generation, due) })
			return nil
		}
		opened := m.beginFestivalLocked(generation, due)
		return func(world World) {
			for _, a := range opened {
				m.openArena(world, a)
			}
			m.settleThen(world, memberIDs(opened), guideName, startingText)
		}
	})
}

func (m *Manager) armLocked(delayMillis int64, fn func()) {
	m.timer = m.afterFunc(time.Duration(delayMillis)*time.Millisecond, fn)
}

func (m *Manager) armFestivalLocked(fest *festivalRun, delayMillis int64, fn func()) {
	fest.timer = m.afterFunc(time.Duration(delayMillis)*time.Millisecond, fn)
}

func (m *Manager) cancelLocked() {
	m.generation++
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}

func (m *Manager) nowMillis() int64 {
	return m.now().UnixMilli()
}

// NextFestivalNotice is the line a festival guide's page shows about the
// next festival. Outside seal validation it counts the minutes left until
// the next festival begins: the milliseconds left divided by a minute,
// truncated toward zero and narrowed through single precision, plus one.
// The count goes negative once the awaited start has passed, and counts
// from the epoch before any schedule has started. Once the last festival
// of a cycle called off by CompetitionEnded has begun, the awaited start
// stays put until CompetitionBegun.
func (m *Manager) NextFestivalNotice() string {
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation {
		return `<font color="FF0000">This is the Seal Validation period. Festivals will resume next week.</font>`
	}
	m.mu.Lock()
	minutes := m.minutesToNextFestivalLocked()
	m.mu.Unlock()
	return `<font color="FF0000">The next festival will begin in ` + strconv.Itoa(minutes) + ` minute(s).</font>`
}

// minutesToNextCycle counts the minutes left until the next festival cycle
// is due: the milliseconds left divided by a minute, truncated toward zero
// and narrowed through single precision; -1 during seal validation. It
// goes negative once the cycle is overdue, and counts from the epoch before
// any schedule has started.
func (m *Manager) minutesToNextCycle() int {
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation {
		return -1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return int(float32((m.nextCycleStart - m.nowMillis()) / 60000))
}
