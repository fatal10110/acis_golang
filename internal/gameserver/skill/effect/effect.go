package effect

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
)

type Effect struct {
	Skill    Skill
	Template modelskill.EffectTemplate
	Type     Type
	Flag     Flag
	Funcs    []basefunc.Func
	Herb     bool
	Effector any
	Effected any

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
// scheduleEffect().
type restoreSeed struct {
	count   int32
	elapsed int32
}

// seedRestore marks e to resume from count and elapsedSeconds on its next
// startSchedule call rather than starting fresh.
func (e *Effect) seedRestore(count, elapsedSeconds int32) {
	if e == nil {
		return
	}
	e.scheduleMu.Lock()
	e.restore = &restoreSeed{count: count, elapsed: elapsedSeconds}
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
	if e == nil || e.Template.Time <= 0 {
		return 0
	}
	return time.Duration(e.Template.Time) * time.Second
}

func (e *Effect) startSchedule(now time.Time) {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()

	if r := e.restore; r != nil {
		e.restore = nil
		e.startScheduleFromRestoreLocked(r, now)
		return
	}

	e.remaining = e.Template.Count
	if period := e.period(); period > 0 {
		e.nextAction = now.Add(period)
		return
	}
	e.nextAction = time.Time{}
}

// startScheduleFromRestoreLocked seeds e.remaining and e.nextAction from a
// persisted tick count and elapsed time, mirroring
// AbstractEffect.setCount(newCount)/setTime(newTime) ahead of a restored
// effect's scheduleEffect() call: the tick count is clamped to the
// template's own count, and the elapsed time (seconds since the effect's
// last tick at logout) is clamped to the template's period before being
// subtracted from it to find the delay until the next tick. Called with
// e.scheduleMu already held.
func (e *Effect) startScheduleFromRestoreLocked(r *restoreSeed, now time.Time) {
	e.remaining = int(min(r.count, int32(e.Template.Count)))

	period := e.period()
	if period <= 0 {
		e.nextAction = time.Time{}
		return
	}

	periodSeconds := int32(e.Template.Time)
	elapsed := min(r.elapsed, periodSeconds)
	delay := max(time.Duration(periodSeconds-elapsed)*time.Second, 0)
	e.nextAction = now.Add(delay)
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

func (e *Effect) stopSchedule() {
	e.scheduleMu.Lock()
	e.remaining = 0
	e.nextAction = time.Time{}
	e.scheduleMu.Unlock()
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
func (e *Effect) beginExit() func() {
	if !e.inUse {
		return nil
	}
	e.inUse = false
	e.stopSchedule()
	if e.OnExit == nil {
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
	e.scheduleMu.Lock()
	next := e.nextAction
	e.scheduleMu.Unlock()

	if e.Template.Count > 1 {
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

	if !next.IsZero() {
		remaining := max(next.Sub(now), 0)
		return int32(remaining.Milliseconds()), true
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
// icon-showing effects, in buffs-then-debuffs order, mirroring
// EffectList.updateEffectIcons()'s buff/debuff scan: an effect not currently
// active, not flagged to show an icon, or classified SIGNET_GROUND is
// skipped, matching the reference's own skip conditions.
func (l *List) IconEntries(now time.Time) []IconEntry {
	var entries []IconEntry
	for _, e := range l.active() {
		if !e.Template.Icon || e.ClassTag() == "SIGNET_GROUND" {
			continue
		}
		duration, ok := e.iconDuration(now)
		if !ok {
			continue
		}
		entries = append(entries, IconEntry{ID: int32(e.Skill.ID), Level: e.Level, Toggle: e.Skill.Toggle, Duration: duration})
	}
	return entries
}

// StatOwner receives stat function changes when active effects change and
// reports the owner's current buff-slot capacity.
