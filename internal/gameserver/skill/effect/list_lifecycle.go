package effect

import (
	"slices"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func (l *List) Tick() {
	l.tickAt(l.now())
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
	l.addAnnounced(e, true)
}

// AddRestored is Add for an effect reinstated at login: it activates e and
// refreshes the icons the same way, but sends the owner none of the
// felt/disappeared/expiry system messages. The reference restores effects
// before the player has a client, so those messages go nowhere; only the
// later icon update reaches the client.
func (l *List) AddRestored(e *Effect) {
	l.addAnnounced(e, false)
}

func (l *List) addAnnounced(e *Effect, announce bool) {
	if l == nil || e == nil {
		return
	}
	var pending []func()
	l.mu.Lock()
	l.silent = !announce
	l.add(e, &pending)
	l.silent = false
	exiting := l.exiting
	l.exiting = nil
	l.mu.Unlock()

	runHooks(pending)
	exits := l.dropDeferred(exiting, announce)
	l.notifyAbnormalUpdate()
	runHooks(exits)
	l.notifyActivityTransition()
}

// Drop removes e, which its own exit hook is ending, when l still holds it —
// as a displaced stack member does with cancel-lesser off. An effect that
// already left the list is ignored. When an Add in progress ran that exit
// hook, the removal waits until the Add's own hooks have run, so the owner
// sees e's removal after the newcomer took its place, and one icon refresh
// covers both.
func (l *List) Drop(e *Effect) {
	if l == nil || e == nil {
		return
	}
	l.mu.Lock()
	if _, ok := l.dropHeld[e]; ok {
		l.dropHeld[e] = true
		l.mu.Unlock()
		return
	}
	held := l.holdsLocked(e)
	l.mu.Unlock()
	if held {
		l.Remove(e)
	}
}

// holdExit records that the add in progress queued held effect e's exit
// hook, so a Drop from that hook waits for the add to finish.
func (l *List) holdExit(e *Effect) {
	if l.dropHeld == nil {
		l.dropHeld = make(map[*Effect]bool)
	}
	l.dropHeld[e] = false
	l.exiting = append(l.exiting, e)
}

// dropDeferred releases the effects an add held for its exit hooks and
// removes those whose hook asked for a Drop. It runs their list bookkeeping
// and returns their exit hooks, to run after the icon refresh. A restored
// add's drops send no system messages, as the add itself sends none.
func (l *List) dropDeferred(exiting []*Effect, announce bool) []func() {
	if len(exiting) == 0 {
		return nil
	}
	var pending, exits []func()
	l.mu.Lock()
	l.silent = !announce
	for _, e := range exiting {
		requested, ok := l.dropHeld[e]
		if !ok {
			continue
		}
		delete(l.dropHeld, e)
		if requested {
			l.remove(e, &pending, &exits)
		}
	}
	l.silent = false
	l.mu.Unlock()
	runHooks(pending)
	return exits
}

// holdsLocked reports whether e is still in l's visible lists or a stack
// queue. Caller must hold l.mu.
func (l *List) holdsLocked(e *Effect) bool {
	return l.contained(e) != nil || slices.Contains(l.stacks[e.stackType()], e)
}

// Remove drops e from the list and activates the next member of its stack
// group when one exists. The owner sees the stat removal, the next member's
// activation and the expiry message, then the icon refresh, and only then
// e's exit hook: an exit hook that sends packets or removes further effects
// does so after e's own removal is announced.
func (l *List) Remove(e *Effect) {
	if l == nil || e == nil {
		return
	}
	var pending, exits []func()
	l.mu.Lock()
	l.remove(e, &pending, &exits)
	l.mu.Unlock()

	runHooks(pending)
	l.notifyAbnormalUpdate()
	runHooks(exits)
	l.notifyActivityTransition()
}

// notifyActivityTransition reconciles l's recorded registration state
// (l.tracked) against its actual current emptiness and calls the activity
// registry if they disagree, in one critical section under l.mu.
//
// It is called after runHooks has already run, rather than deciding the
// transition immediately after add/remove: a queued OnStart hook can still
// reject the effect (removeFromVisible), draining a list that looked
// active right back to empty before the caller returns. Reading the
// settled state here means a rejected effect never registers a phantom
// empty list, and a hook that reactivates a stack member never
// deregisters a list that's actually still live.
//
// Comparing against l.tracked instead of a wasEmpty value the caller
// captured before releasing mu — and calling the registry without releasing
// mu in between — matters because Add and Remove run concurrently on the
// same list in the ordinary case (Effects.Tick draining an expiring
// effect on its own goroutine while a skill lands a new one on another).
// Two independently-locked "decide, then apply" halves can interleave so
// the later apply overwrites the earlier one's registry state with a
// stale decision, permanently registering an empty list or, worse,
// leaving a live one unregistered forever. Deciding and applying inside
// one lock hold serializes concurrent transitions through l.mu itself, so
// whichever call enters second always reconciles against the first call's
// already-applied result instead of a snapshot taken before it ran.
// The registry only takes its own separate mutex and never
// re-enters List, so calling it here does not risk l.mu deadlocking
// against itself — but it does establish an l.mu -> registry.mu lock
// order that any future caller on the registry side must not invert.
func (l *List) notifyActivityTransition() {
	l.mu.Lock()
	defer l.mu.Unlock()

	active := !l.emptyLocked() && !l.untracked
	if active == l.tracked {
		return
	}
	l.tracked = active
	if l.activity != nil {
		l.activity.SetActive(l, active)
	}
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

// StopBySkillID removes every held effect the given skill applied, running
// each removed instance's exit hook, matching Creature.stopSkillEffects(int).
func (l *List) StopBySkillID(id modelskill.ID) {
	if l == nil {
		return
	}
	for _, e := range l.All() {
		if e.Skill.ID == id {
			l.Remove(e)
		}
	}
}

// StopAllToggles removes every toggle held among the buffs, running each
// exit hook, matching EffectList.stopAllToggles.
func (l *List) StopAllToggles() {
	if l == nil {
		return
	}
	l.mu.Lock()
	buffs := slices.Clone(l.buffs)
	l.mu.Unlock()
	for _, e := range buffs {
		if e != nil && e.Skill.Toggle {
			l.Remove(e)
		}
	}
}

// StopAll removes every active effect, running each exit hook.
func (l *List) StopAll() {
	if l == nil {
		return
	}
	for _, e := range l.All() {
		l.Remove(e)
	}
}

// StopAllExceptThoseThatLastThroughDeath removes active effects whose owning
// skill is not configured to persist through death.
func (l *List) StopAllExceptThoseThatLastThroughDeath() {
	if l == nil {
		return
	}
	for _, e := range l.All() {
		if !e.Skill.StayAfterDeath {
			l.Remove(e)
		}
	}
}

// notifyAbnormalUpdate tells l's owner to refresh its abnormal-effect icon
// state, mirroring Creature.addEffect()/removeEffect() unconditionally
// queueing an EffectList icon update on every add or remove attempt,
// regardless of whether the attempt actually changed anything.
func (l *List) notifyAbnormalUpdate() {
	if l.owner != nil {
		l.owner.UpdateEffectIcons()
	}
}

// notifyExpiry queues e's worn-off/disappeared/aborted system message,
// mirroring EffectList.removeEffectFromQueue: only for an effect that
// actually left the buffs/debuffs list (not one rejected before insertion)
// and whose template shows an icon. wornOff must be read before
// e.stopSchedule() runs, since that call zeroes e's remaining-tick counter.
// e.Skill.Toggle wins over the count check even though a toggle's schedule
// never reaches count 0, matching the reference checking isToggle() first.
func (l *List) notifyExpiry(e *Effect, wornOff bool, pending *[]func()) {
	if !e.Template.Icon || l.silent {
		return
	}
	notifier := l.owner
	if notifier == nil {
		return
	}
	skillID, level := e.Skill.ID, e.Skill.Level
	switch {
	case e.Skill.Toggle:
		*pending = append(*pending, func() { notifier.NotifyEffectAborted(skillID, level) })
	case wornOff:
		*pending = append(*pending, func() { notifier.NotifyEffectWornOff(skillID, level) })
	default:
		*pending = append(*pending, func() { notifier.NotifyEffectDisappeared(skillID, level) })
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
// runs (still under l.mu) on failure. With announce set, a successful
// activation of an icon effect then tells the owner it feels e's effect —
// the add path's stack-head promotion does this (unless l.silent, read here
// under l.mu), the removal path's does not.
//
// It does not (re)start e's tick schedule: that starts once, in add, when e
// is first created — matching L2Skill.getEffects() calling scheduleEffect()
// unconditionally for every created effect (L2Skill.java:1188-1191) — so a
// promoted stack loser resumes with whatever count it drained down to while
// displaced instead of restarting from the template.
func (l *List) beginActivate(e *Effect, onReject func(*Effect), announce bool) func() {
	announce = announce && !l.silent
	return func() {
		ok := true
		if e.OnStart != nil {
			ok = e.OnStart(e)
		}

		l.mu.Lock()
		if ok {
			e.inUse = true
			if !e.startRefused {
				l.addStatFuncs(e)
			}
		} else {
			onReject(e)
		}
		l.mu.Unlock()

		if ok && announce && !e.startRefused && e.Template.Icon && l.owner != nil {
			l.owner.NotifyEffectFelt(e.Skill.ID, e.Skill.Level)
		}
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
//
// A buff e replaces (identical) or evicts (buff-slot cap) is retired in two
// steps. Its stop-task and exit hooks are queued at once, but it stays held
// — counted, stacked and positioned — for the rest of e's insertion, and its
// list removal (stat removal, next-member promotion, expiry message) is
// queued only after e's own activation. The owner therefore sees the old
// exit hook, the newcomer's start, then the old removal and its message,
// then the icon refresh.
func (l *List) add(e *Effect, pending *[]func()) {
	e.startSchedule(l.now())

	if e.RejectsIfAffected && l.flagsLocked()&e.Flag != 0 {
		appendThunk(pending, e.stopTaskThunk())
		return
	}

	var retiring []*Effect
	l.insert(e, pending, &retiring)
	for _, old := range retiring {
		l.holdExit(old)
		l.remove(old, pending, pending)
	}
}

// insert places e in its visible list and stack group, retiring the buffs it
// replaces or evicts into retiring (see add).
func (l *List) insert(e *Effect, pending *[]func(), retiring *[]*Effect) {
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
				retire(existing, pending, retiring)
			}
		}

		// Herbs never evict a real buff: at or over capacity, they are
		// simply dropped. A buff retired above still counts here.
		if e.Herb && l.buffCount() >= l.maxBuffCount() {
			appendThunk(pending, e.stopTaskThunk())
			return
		}

		if !l.doesStack(e) && !e.Skill.sevenSigns() {
			l.evictForCap(e, pending, retiring)
		}

		l.insertBuff(e)
	}

	if e.stackType() == "none" {
		*pending = append(*pending, l.beginActivate(e, func(rejected *Effect) { l.removeFromVisible(rejected) }, false))
		return
	}

	l.addStacked(e, pending)
}

// retire queues e's stop-task hook and, if e is active, its exit hook, and
// records e for removal once the insertion that retired it completes. A
// second retirement of the same effect (a cap eviction reaching a buff the
// identical check already retired) queues nothing more.
func retire(e *Effect, pending *[]func(), retiring *[]*Effect) {
	if slices.Contains(*retiring, e) {
		return
	}
	appendThunk(pending, e.stopTaskThunk())
	appendThunk(pending, e.finishExit())
	*retiring = append(*retiring, e)
}
