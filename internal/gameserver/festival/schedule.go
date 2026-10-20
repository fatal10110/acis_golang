package festival

import (
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
// opens the next signup period at once, for as long as nobody signs up.
type schedule struct {
	// nextFestivalStart is in Unix milliseconds; zero until a schedule
	// first sets it.
	nextFestivalStart int64
	// nextCycleStart is when the next festival cycle is due, in Unix
	// milliseconds: a cycle length after the schedule started or the last
	// signup period opened; zero until a schedule first sets it.
	nextCycleStart int64
	timer          *time.Timer
	// generation tells a timer of a cancelled schedule from the running
	// one's: each start and cancel moves it on.
	generation uint64
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

// Stop cancels the schedule for good.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	m.cancelLocked()
}

// CompetitionBegun starts a new festival schedule in place of any running
// one and counts a new festival cycle.
func (m *Manager) CompetitionBegun() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.cancelLocked()
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
// points, once per festival it holds the record of. A festival under way
// runs to its end first, its parties taken back out of their arenas; no
// cycle follows it. The awaited festival start
// stays where the last signup period put it until CompetitionBegun starts
// a new schedule, so NextFestivalNotice counts down past zero through the
// results period and, in the recruiting period that follows seal
// validation, shows about a week's worth of minutes below zero. Nothing
// keeps moving the start after the stop, so the guides never show a fresh
// 1 to 20 minute countdown in those periods.
func (m *Manager) CompetitionEnded() {
	m.mu.Lock()
	if m.initialized {
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
		return
	}
	for _, name := range names {
		world.RewardClanOf(name, highestRankedReward)
	}
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
func (m *Manager) signupEnded(generation uint64, due int64) {
	m.step(generation, func() func(World) {
		m.previous = [2]roster{{}, {}}
		if len(m.participants[0]) == 0 && len(m.participants[1]) == 0 {
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
			world.Settle(memberIDs(opened))
			world.Shout(guideName, startingText)
		}
	})
}

func (m *Manager) armLocked(delayMillis int64, fn func()) {
	m.timer = m.afterFunc(time.Duration(delayMillis)*time.Millisecond, fn)
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
// from the epoch before any schedule has started. Between CompetitionEnded
// and the next CompetitionBegun the awaited start does not move: see
// CompetitionEnded for what the guides show then.
func (m *Manager) NextFestivalNotice() string {
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation {
		return `<font color="FF0000">This is the Seal Validation period. Festivals will resume next week.</font>`
	}
	m.mu.Lock()
	minutes := int(float32((m.nextFestivalStart-m.nowMillis())/60000)) + 1
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
