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
	timer             *time.Timer
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
	m.status.FestivalCycle++
	now := m.nowMillis()
	start := m.cfg.ManagerStart.Milliseconds()
	m.nextFestivalStart = now + start + m.cfg.signup().Milliseconds()
	generation, due := m.generation, now+start
	m.armLocked(start, func() { m.runCycle(generation, due) })
	m.log.Info().Int64("minutes", start/60000).Msg("festival: the first festival cycle begins")
}

// CompetitionEnded stops the festival schedule. The awaited festival start
// stays where the last signup period put it until CompetitionBegun starts
// a new schedule, so NextFestivalNotice counts down past zero through the
// results period and, in the recruiting period that follows seal
// validation, shows about a week's worth of minutes below zero. The
// reference's no-party wait loop outlives the cancel and keeps moving the
// start, so its guides show 1 to 20 minutes in both periods; that loop is
// not reproduced.
func (m *Manager) CompetitionEnded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelLocked()
	// ponytail: the end of the competition also rewards the clans of the
	// best-ever party of each festival (#223); no score can be set yet.
}

// runCycle runs the festival cycle that was due at due. During seal
// validation, or when the Seven Signs period ends before a whole cycle
// could run, it skips this cycle and waits for the next one, a cycle
// length after due.
func (m *Manager) runCycle(generation uint64, due int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if generation != m.generation {
		return
	}
	now := m.nowMillis()
	cycle := m.cfg.CycleLength.Milliseconds()
	if m.calendar.CurrentPeriod() == sevensigns.SealValidation || m.calendar.NextChange().UnixMilli()-now < cycle {
		next := due + cycle
		m.armLocked(max(0, next-now), func() { m.runCycle(generation, next) })
		return
	}
	m.armLocked(m.cfg.signup().Milliseconds(), func() { m.signupEnded(generation) })
}

// signupEnded ends a signup period. No party can sign up yet (#223), so
// no festival starts: the next signup period opens at once and ends a
// festival's length and a minute later.
func (m *Manager) signupEnded(generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if generation != m.generation {
		return
	}
	now := m.nowMillis()
	wait := (m.cfg.CycleLength - m.cfg.signup()).Milliseconds()
	m.nextFestivalStart = now + wait
	m.armLocked(wait, func() { m.signupEnded(generation) })
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
