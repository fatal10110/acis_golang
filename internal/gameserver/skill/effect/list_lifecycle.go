package effect

import (
	"slices"
	"time"
)

func (l *List) Tick() {
	l.tickAt(time.Now())
}

// tickAt claims a due action from every held effect, active or displaced —
// AbstractEffect.scheduleEffect()'s ACTING case runs (and decrements _count)
// for every scheduled effect regardless of getInUse() (AbstractEffect.java:
// 291-306) — but only runs the periodic hook for the currently active member
// of each stack group; a displaced member silently drains its count and
// self-removes on exhaustion without ever firing its action.
func (l *List) tickAt(now time.Time) {
	for _, e := range l.All() {
		run, remove := e.claimAction(now)
		if !run {
			if remove {
				l.Remove(e)
			}
			continue
		}
		if e.InUse() && !e.ActionTime() {
			remove = true
		}
		if remove {
			l.Remove(e)
		}
	}
}

// Add inserts e and activates it when it wins its stack group.
func (l *List) Add(e *Effect) {
	if l == nil || e == nil {
		return
	}
	var pending []func()
	l.mu.Lock()
	l.add(e, &pending)
	l.mu.Unlock()

	runHooks(pending)
	l.notifyAbnormalUpdate()
}

// Remove drops e from the list and activates the next member of its stack
// group when one exists.
func (l *List) Remove(e *Effect) {
	if l == nil || e == nil {
		return
	}
	var pending []func()
	l.mu.Lock()
	l.remove(e, &pending)
	l.mu.Unlock()

	runHooks(pending)
	l.notifyAbnormalUpdate()
}

// StopByType removes every active effect of the given type, running each
// removed instance's exit hook, matching Creature.stopEffects(EffectType).
func (l *List) StopByType(t Type) {
	if l == nil {
		return
	}
	for _, e := range l.All() {
		if e.Type == t {
			l.Remove(e)
		}
	}
}

// notifyAbnormalUpdate tells l's owner to refresh its abnormal-effect icon
// state, mirroring Creature.addEffect()/removeEffect() unconditionally
// queueing an EffectList icon update on every add or remove attempt,
// regardless of whether the attempt actually changed anything. An owner
// that doesn't track abnormal-effect icons (not a Player) leaves this a
// no-op.
func (l *List) notifyAbnormalUpdate() {
	if u, ok := l.owner.(abnormalUpdater); ok {
		u.UpdateAbnormalEffect()
	}
}

// runHooks fires each queued hook in order, after the caller has released
// l.mu. Add/Remove queue every OnStart/OnExit/OnStopTask call (and the
// owner stat-func callbacks that accompany them) here instead of firing
// them while l.mu is held, so a hook that calls back into this same List's
// Add/Remove doesn't self-deadlock on l.mu (sync.Mutex isn't reentrant).
// Queuing preserves the original call order exactly, since every entry is
// appended at the point the original code invoked it synchronously.
func runHooks(pending []func()) {
	for _, fn := range pending {
		fn()
	}
}

// appendThunk queues thunk, a possibly-nil hook invocation (nil meaning the
// effect has no such hook set).
func appendThunk(pending *[]func(), thunk func()) {
	if thunk != nil {
		*pending = append(*pending, thunk)
	}
}

// beginActivate returns a thunk that runs e's on-start hook once the
// caller's lock is released, then briefly re-acquires l.mu to apply the
// result: e activates and gains its stat funcs on success, or onReject
// runs (still under l.mu) on failure. It does not (re)start e's tick
// schedule: that starts once, in add, when e is first created — matching
// L2Skill.getEffects() calling scheduleEffect() unconditionally for every
// created effect (L2Skill.java:1188-1191) — so a promoted stack loser
// resumes with whatever count it drained down to while displaced instead of
// restarting from the template.
func (l *List) beginActivate(e *Effect, onReject func(*Effect)) func() {
	return func() {
		ok := true
		if e.OnStart != nil {
			ok = e.OnStart(e)
		}

		l.mu.Lock()
		if ok {
			e.inUse = true
			l.addStatFuncs(e)
		} else {
			onReject(e)
		}
		l.mu.Unlock()
	}
}

// add inserts e, starting its tick schedule unconditionally first — matching
// how the reference schedules every created effect's periodic task up front,
// before any stacking/activation outcome is known. A RejectsIfAffected
// effect that finds its own Flag bit
// already set by any currently held effect is dropped outright before any
// buff/debuff handling: its stop-task hook fires and it never reaches the
// identical-effect replace/reject logic below, so a same-skill-id recast
// while the flag is already active is rejected here rather than treated as
// a replacement.
func (l *List) add(e *Effect, pending *[]func()) {
	e.startSchedule(time.Now())

	if e.RejectsIfAffected && l.flagsLocked()&e.Flag != 0 {
		appendThunk(pending, e.stopTaskThunk())
		return
	}

	if e.Skill.Debuff {
		for _, existing := range l.debuffs {
			if existing.identical(e) {
				appendThunk(pending, e.stopTaskThunk())
				return
			}
		}
		l.debuffs = append(l.debuffs, e)
	} else {
		for _, existing := range slices.Clone(l.buffs) {
			if existing.identical(e) {
				l.exit(existing, pending)
			}
		}

		// Herbs never evict a real buff: at or over capacity, they are
		// simply dropped.
		if e.Herb && l.buffCount() >= l.maxBuffCount() {
			appendThunk(pending, e.stopTaskThunk())
			return
		}

		if !l.doesStack(e) && !e.Skill.sevenSigns() {
			l.evictForCap(e, pending)
		}

		l.insertBuff(e)
	}

	if e.stackType() == "none" {
		*pending = append(*pending, l.beginActivate(e, func(rejected *Effect) { l.removeFromVisible(rejected) }))
		return
	}

	l.addStacked(e, pending)
}

// exit fully retires e: its scheduled task is stopped and it is detached
// from stats/visibility and, if active, run through its on-exit hook.
func (l *List) exit(e *Effect, pending *[]func()) {
	appendThunk(pending, e.stopTaskThunk())
	l.remove(e, pending)
}

// doesStack reports whether e's stack type already has a buff member among
// the current stack group, mirroring the check that exempts stacking buffs
// from buff-slot cap eviction. Only called from the non-debuff insertion
// path, it looks at buff members exclusively — a debuff sharing the same
// stack-type string (the shared l.stacks map holds both families) doesn't
// count.
