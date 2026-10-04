package effect

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type Effect struct {
	Skill    Skill
	Template modelskill.EffectTemplate
	Type     Type
	Flag     Flag
	Funcs    []Mod
	Herb     bool
	// SelfTarget holds this effect on its effector while Effected remains the
	// skill target for its hooks.
	SelfTarget bool
	Effector   Actor
	Effected   Actor

	// RejectsIfAffected marks an effect that must not be added at all
	// (only its stop-task hook runs) when the owner is already affected by
	// its own Flag bit from any currently held effect — not just another
	// instance of the same kind. Most kinds leave this false.
	RejectsIfAffected bool

	// Level is the applied skill level this effect instance represents,
	// initialized from Skill.Level. Every kind treats it as fixed for the
	// effect's lifetime except a fusion effect's IncreaseEffect/
	// DecreaseForce, which grow or shrink it while the effect stays live.
	Level int

	OnStart    func(*Effect) bool
	OnAction   func(*Effect) bool
	OnExit     func(*Effect)
	OnStopTask func(*Effect)

	// landing is the knockback effect kind's geo-resolved touchdown point,
	// computed once in throwUpStart and applied in throwUpExit. Unused by
	// every other kind.
	landing location.Location

	inUse bool
	// startRefused marks a kind whose start always fails while the effect
	// stays held: TypeProtectionBless. It is still marked in use (the in-use
	// flag is set before the start runs), so its flag counts, but the failed
	// start leaves its start conditions unmet: activation adds no stat funcs
	// and sends no felt message, and ending it by expiry, dispel, death or
	// replacement skips onExit. Only losing its stack group's head runs
	// onExit, because clearing the in-use flag calls it unconditionally.
	startRefused bool

	// strippedAll marks an effect a stop-all is ending. Its holder removes
	// its stat funcs without reporting the change (see ModOwner.Stripped):
	// the stop-all's caller refreshes the holder's view once, when the
	// strip ends. Read by the holder outside the list lock.
	strippedAll atomic.Bool

	// scheduleMu guards remaining and nextAction. The caster that adds or
	// dispels this effect starts or stops its schedule from the caster's
	// queue, while the owner's effect tick claims actions (claimAction).
	scheduleMu sync.Mutex
	remaining  int
	nextAction time.Time
	// restore, when set, tells the next startSchedule call to resume from a
	// persisted tick count and elapsed time instead of starting fresh from
	// the template, for an effect reinstated by a relog restore. Consumed
	// (set back to nil) the first time startSchedule runs.
	restore *restoreSeed
}

// restoreSeed carries the tick count and time-since-last-tick a persisted
// effect had at logout, mirroring the effect_count/effect_cur_time columns
// AbstractEffect.setCount/setTime seed before Player.restoreEffects() calls
// scheduleEffect(), and the instant at which they were read back: the
// schedule runs from there, not from when the effect joins a list.
type restoreSeed struct {
	count   int32
	elapsed int32
	// at is the restore instant; zero means the schedule starts.
	at time.Time
}

// seedRestore marks e to resume from count and elapsedSeconds, as of the
// restore instant at (zero: as of the schedule start), on its next
// startSchedule call rather than starting fresh.
func (e *Effect) seedRestore(count, elapsedSeconds int32, at time.Time) {
	if e == nil {
		return
	}
	e.scheduleMu.Lock()
	e.restore = &restoreSeed{count: count, elapsed: elapsedSeconds, at: at}
	e.scheduleMu.Unlock()
}

// InUse reports whether e is the active member of its stack group.
func (e *Effect) InUse() bool {
	if e == nil {
		return false
	}
	return e.inUse
}

// Remaining reports the scheduler's remaining-tick counter. On the first
// action of a Count N effect, it is N-1, then decreases on later actions.
func (e *Effect) Remaining() int {
	if e == nil {
		return 0
	}
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	return e.remaining
}

// ActionTime runs e's periodic hook. Effects without periodic behavior stop
// after one action tick.
func (e *Effect) ActionTime() bool {
	if e == nil || e.OnAction == nil {
		return false
	}
	return e.OnAction(e)
}

func (e *Effect) period() time.Duration {
	if e == nil {
		return 0
	}
	return templatePeriod(e.Template)
}

// templatePeriod is the tick period of an effect built from tmpl; zero for
// an effect without one.
func templatePeriod(tmpl modelskill.EffectTemplate) time.Duration {
	if tmpl.Time <= 0 {
		return 0
	}
	return time.Duration(tmpl.Time) * time.Second
}

// initialCount is the tick count a fresh schedule starts from: the
// template's own, except a fear from one of the halved-duration skills landing
// on a playable, which starts at half of it. The template itself is left
// alone, so the icon's repeat-count check and a relog restore's clamp still
// read the configured count.
func (e *Effect) initialCount() int {
	if e.Type == TypeFear && isPlayable(e.Effected) && fearHalvedDurationPlayableSkillIDs[e.Skill.ID] {
		return e.Template.Count / 2
	}
	return e.Template.Count
}

func (e *Effect) startSchedule(now time.Time) {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()

	if r := e.restore; r != nil {
		e.restore = nil
		e.startScheduleFromRestoreLocked(r, now)
		return
	}

	e.remaining = e.initialCount()
	if period := e.period(); period > 0 {
		e.nextAction = now.Add(period)
		return
	}
	e.nextAction = time.Time{}
}

// startScheduleFromRestoreLocked seeds e.remaining and e.nextAction from a
// persisted tick count and elapsed time, run on from the restore instant. An
// effect with a tick action keeps a first tick already due: List.Restore
// runs the actions of every tick due by now. One without ends on the next
// tick, running no action, when its first tick came due before now. Called
// with e.scheduleMu already held.
func (e *Effect) startScheduleFromRestoreLocked(r *restoreSeed, now time.Time) {
	at := r.at
	if at.IsZero() {
		at = now
	}
	if e.OnAction != nil {
		e.remaining, e.nextAction = restoredSchedule(e.Template, r.count, r.elapsed, at)
		return
	}
	remaining, next, ok := resumeRestored(e.Template, r.count, r.elapsed, at, now)
	if !ok {
		e.remaining, e.nextAction = 0, now
		return
	}
	e.remaining, e.nextAction = remaining, next
}

// restoredSchedule is the tick count and first tick of an effect built from
// tmpl and restored at the instant at from a persisted tick count and
// elapsed time: the tick count is clamped to the template's own count, and
// the elapsed time (seconds since the effect's last tick at logout) is
// clamped to the template's period, so the first tick comes due that much
// short of a period after at, but never sooner than restoreMinDelay after
// it: a tick saved overdue is not due at the restore instant itself. An
// effect without a period only has its count clamped.
func restoredSchedule(tmpl modelskill.EffectTemplate, count, elapsedSeconds int32, at time.Time) (remaining int, next time.Time) {
	remaining = int(min(count, int32(tmpl.Count)))
	period := templatePeriod(tmpl)
	if period <= 0 {
		return remaining, time.Time{}
	}
	elapsed := min(time.Duration(elapsedSeconds)*time.Second, period)
	return remaining, at.Add(max(period-elapsed, restoreMinDelay))
}

// restoreMinDelay is the least delay before a restored effect's first tick,
// the reference's minimum initial delay for a resumed effect task.
const restoreMinDelay = 5 * time.Millisecond

// resumeRestored is the schedule, at now, of an effect built from tmpl whose
// ticks run no action, restored at the instant at (see restoredSchedule).
// Such an effect ends on its first tick whatever count it has left, as
// List.tickAt ends an in-use effect whose action reports false: a tick due
// before now has ended it, and ok is false. (A restored effect stacked out
// by another restored one would keep counting in the reference instead;
// both are restored debuffs of one stack group, rare enough to leave
// unmodelled.)
func resumeRestored(tmpl modelskill.EffectTemplate, count, elapsedSeconds int32, at, now time.Time) (remaining int, next time.Time, ok bool) {
	remaining, next = restoredSchedule(tmpl, count, elapsedSeconds, at)
	if !next.IsZero() && next.Before(now) {
		return 0, time.Time{}, false
	}
	return remaining, next, true
}

// RestoredSaveState is the tick count and elapsed seconds an effect that a
// login restored at the instant at, from count and elapsedSeconds, but has
// not replayed yet saves at now: its schedule ran from at, as the replayed
// effect's own does (see resumeRestored). The first template ApplyRestored
// would build whose effect is still running at now is the one whose state
// the replayed skill saves, as a save of the live list writes the first
// effect of a skill still in it: one that has ended passes the row on to the
// next. ok is false only when every such effect has ended by now. A zero at,
// a skill with no such template, or a first running one whose ticks run an
// action, saves count and elapsedSeconds unchanged: only a replay runs those
// actions (ApplyRestored), and a save that skipped it must not spend their
// ticks without them. A drop on the loading screen replays the staged
// effects before it saves, so this is the save of effects no replay reached.
func RestoredSaveState(templates []modelskill.EffectTemplate, count, elapsedSeconds int32, at, now time.Time) (savedCount, savedElapsed int32, ok bool) {
	if at.IsZero() {
		return count, elapsedSeconds, true
	}
	known := false
	for _, tmpl := range templates {
		if _, ok := coreKinds[tmpl.Name]; !ok {
			continue
		}
		known = true
		period := templatePeriod(tmpl)
		if period <= 0 || actsOnTick(tmpl) {
			return count, elapsedSeconds, true
		}
		remaining, next, alive := resumeRestored(tmpl, count, elapsedSeconds, at, now)
		if !alive {
			continue
		}
		left := min(max(next.Sub(now), 0), period)
		return int32(remaining), int32((period - left) / time.Second), true
	}
	if known {
		return 0, 0, false
	}
	return count, elapsedSeconds, true
}

// SaveState reports the tick count and elapsed-seconds-since-last-tick e
// should persist at now, the inverse of startScheduleFromRestoreLocked:
// elapsed is the template period minus the time remaining until e's next
// scheduled tick, clamped to [0, period]. An effect with no period (a
// single unscheduled or permanent effect) reports zero elapsed.
func (e *Effect) SaveState(now time.Time) (count, elapsed int32) {
	if e == nil {
		return 0, 0
	}
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()

	count = int32(e.remaining)
	period := e.period()
	if period <= 0 || e.nextAction.IsZero() {
		return count, 0
	}
	remaining := min(max(e.nextAction.Sub(now), 0), period)
	return count, int32((period - remaining) / time.Second)
}

// periodRemaining reports the whole seconds left in e's current tick period
// at now: the template period minus the whole seconds elapsed since the
// period started. An effect with no running schedule reports its full
// template period.
func (e *Effect) periodRemaining(now time.Time) int {
	_, elapsed := e.SaveState(now)
	return e.Template.Time - int(elapsed)
}

func (e *Effect) stopSchedule() {
	e.scheduleMu.Lock()
	e.remaining = 0
	e.nextAction = time.Time{}
	e.scheduleMu.Unlock()
}

// dueAt is the instant e's next tick comes due, zero when none is
// scheduled.
func (e *Effect) dueAt() time.Time {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	return e.nextAction
}

func (e *Effect) claimAction(now time.Time) (runAction bool, remove bool) {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()

	if e.nextAction.IsZero() || now.Before(e.nextAction) {
		return false, false
	}
	if e.remaining <= 0 {
		e.nextAction = time.Time{}
		return false, true
	}

	e.remaining--
	remove = e.remaining <= 0
	if remove {
		e.nextAction = time.Time{}
	} else {
		e.nextAction = e.nextAction.Add(e.period())
	}
	return true, remove
}

// beginExit flips e's in-use flag off, if it was on, and returns a thunk
// that fires the resulting on-exit hook — or nil if e wasn't active or has
// no such hook. The flag flips immediately (so InUse() is accurate the
// moment the caller's lock is released) but the hook itself is returned
// for the caller to run later, outside that lock: see List.runHooks.
//
// It does not stop e's tick schedule: a stacked-out loser keeps draining its
// own count while displaced. Every tick decrements the count whether or not
// the effect is in use, and the schedule only stops once the count is
// exhausted or the effect actually leaves the list — see List.remove.
func (e *Effect) beginExit() func() {
	if !e.inUse {
		return nil
	}
	e.inUse = false
	if e.OnExit == nil {
		return nil
	}
	return func() { e.OnExit(e) }
}

// finishExit is beginExit for an effect that is ending for good (expiry,
// dispel, removal). A startRefused effect runs no exit hook there and stays
// marked in use. A buff an insertion replaces or evicts ends through
// retireExit instead.
func (e *Effect) finishExit() func() {
	if e.startRefused {
		return nil
	}
	return e.beginExit()
}

// retireExit returns the exit hook one ending pass over a buff an insertion
// replaces or evicts runs, or nil when e is inactive, start-refused or has
// no hook. Unlike finishExit it leaves e marked in use, because the
// insertion still holds e and every later pass over it runs the hook again
// while e stays active: a cap eviction reaching a buff the identical check
// already retired, then the stack-head change (beginExit), which clears the
// flag. add clears it for good before e's list removal, which runs no hook.
func (e *Effect) retireExit() func() {
	if !e.inUse || e.startRefused || e.OnExit == nil {
		return nil
	}
	return func() { e.OnExit(e) }
}

// stopTaskThunk returns a thunk that fires e's on-stop-task hook, or nil
// when e has none. Like beginExit, the caller runs the returned thunk only
// after releasing List's lock.
func (e *Effect) stopTaskThunk() func() {
	if e.OnStopTask == nil {
		return nil
	}
	return func() { e.OnStopTask(e) }
}

func (e *Effect) identical(other *Effect) bool {
	if e == nil || other == nil {
		return false
	}
	return e.Skill.ID == other.Skill.ID &&
		e.Type == other.Type &&
		e.Template.StackOrder == other.Template.StackOrder &&
		e.Template.StackType == other.Template.StackType
}

func (e *Effect) stackType() string {
	if e == nil || e.Template.StackType == "" {
		return "none"
	}
	return e.Template.StackType
}

// iconDuration reports the remaining duration, in milliseconds, e should
// report to an AbnormalStatusUpdate-style icon list, mirroring
// AbstractEffect.addIcon(): a repeat-count effect reports its remaining tick
// countdown, a single scheduled effect reports the time left until that
// schedule fires, and a permanent (no period) effect reports -1. ok is false
// when none of those apply (an unscheduled, non-repeating, non-permanent
// effect), meaning it is omitted from the icon list entirely.
func (e *Effect) iconDuration(now time.Time) (millis int32, ok bool) {
	if e.Template.Count > 1 {
		e.scheduleMu.Lock()
		next := e.nextAction
		e.scheduleMu.Unlock()
		// Mirrors AbstractEffect.addIcon's repeat-count branch: elapsed is
		// the whole seconds since the current tick's period started, so the
		// value decrements every second instead of holding flat for a whole
		// tick.
		var elapsed int64
		if period := e.period(); period > 0 && !next.IsZero() {
			elapsed = int64(now.Sub(next.Add(-period)) / time.Second)
		}
		return int32((int64(e.Remaining())*int64(e.Template.Time) - elapsed) * 1000), true
	}
	return e.partyIconDuration(now)
}

// partyIconDuration is iconDuration without the repeat-count countdown: a
// scheduled effect reports the time left until its next action, an
// unscheduled permanent one -1, and any other is left out.
func (e *Effect) partyIconDuration(now time.Time) (millis int32, ok bool) {
	e.scheduleMu.Lock()
	next := e.nextAction
	e.scheduleMu.Unlock()

	if !next.IsZero() {
		return int32(max(next.Sub(now), 0).Milliseconds()), true
	}
	if e.Template.Time == -1 {
		return -1, true
	}
	return 0, false
}

// IconEntry is one active effect's projection onto an AbnormalStatusUpdate-
// style icon list.
type IconEntry struct {
	ID       int32
	Level    int
	Toggle   bool
	Duration int32
}

// IconEntries returns the icon-list projection of l's currently active,
// icon-showing effects, in buffs-then-debuffs order: an effect not currently
// active, not flagged to show an icon, or classified SIGNET_GROUND is
// skipped.
func (l *List) IconEntries(now time.Time) []IconEntry {
	return l.iconEntries(now, (*Effect).iconDuration)
}

// PartyIconEntries is IconEntries for the icon list a party sees of the
// owner (PartySpelled): the same effects, but a scheduled effect always
// reports the time until its next action, repeat count or not, and an
// unscheduled one only when permanent (-1).
func (l *List) PartyIconEntries(now time.Time) []IconEntry {
	return l.iconEntries(now, (*Effect).partyIconDuration)
}

func (l *List) iconEntries(now time.Time, durationOf func(*Effect, time.Time) (int32, bool)) []IconEntry {
	var entries []IconEntry
	for _, e := range l.active() {
		if !e.Template.Icon || e.ClassTag() == "SIGNET_GROUND" {
			continue
		}
		duration, ok := durationOf(e, now)
		if !ok {
			continue
		}
		entries = append(entries, IconEntry{ID: int32(e.Skill.ID), Level: e.iconLevel(), Toggle: e.Skill.Toggle, Duration: duration})
	}
	return entries
}

// StatOwner receives stat function changes when active effects change and
// reports the owner's current buff-slot capacity.
