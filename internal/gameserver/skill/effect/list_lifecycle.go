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
// a scheduled tick runs (and decrements the count) for every scheduled
// effect whether or not it is in use — but only runs the periodic hook for
// the currently active member
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

// AddRestored is Add for an effect reinstated at login: it activates e the
// same way, but sends the owner none of the felt/disappeared/expiry system
// messages. Effects are restored before the player has a client, so those
// messages go nowhere; only the later icon update reaches the client.
//
// It runs as a Restore of its own unless one is open (see Restore): once e
// is active, the actions of its ticks already due run in order before the
// one icon refresh, so an effect they end is not in it.
func (l *List) AddRestored(e *Effect) {
	l.Restore(func() { l.addAnnounced(e, false) })
}

// restoreBatch is what one Restore collects from the restored adds it runs:
// the effects they added, and the exit hooks their insertions queued, to
// run after its icon refresh as an Add runs them after its own.
type restoreBatch struct {
	added []*Effect
	exits []func()
}

// Restore runs apply, whose AddRestored calls join l without their icon
// refreshes, then the actions of the ticks of those effects that came due
// before now, in time order across all of them (see catchUp), and then
// refreshes the icons once. The reference schedules every restored effect
// at the restore instant on its own fixed-rate task, so the ticks due since
// then interleave by time, not by the order the effects were saved in. A
// Restore inside another one just runs apply: the outermost one catches up
// and refreshes for all of them. Call it on the owner's queue.
func (l *List) Restore(apply func()) {
	if l == nil {
		apply()
		return
	}
	l.mu.Lock()
	if l.restoring != nil {
		l.mu.Unlock()
		apply()
		return
	}
	b := &restoreBatch{}
	l.restoring = b
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.restoring = nil
		l.mu.Unlock()
	}()

	apply()

	l.mu.Lock()
	added, exits := b.added, b.exits
	l.mu.Unlock()
	if len(added) == 0 {
		runHooks(exits)
		return
	}
	exits = append(exits, l.catchUp(added)...)
	l.notifyAbnormalUpdate()
	runHooks(exits)
	l.notifyActivityTransition()
}

func (l *List) addAnnounced(e *Effect, announce bool) {
	if l == nil || e == nil || (l.admit != nil && !l.admit(e)) {
		return
	}
	var pending []func()
	l.mu.Lock()
	l.silent = !announce
	l.add(e, &pending)
	l.silent = false
	exiting := l.exiting
	l.exiting = nil
	var batch *restoreBatch
	if !announce {
		batch = l.restoring
	}
	l.mu.Unlock()

	runHooks(pending)
	exits := l.dropDeferred(exiting, announce)
	if batch != nil {
		l.mu.Lock()
		batch.added = append(batch.added, e)
		batch.exits = append(batch.exits, exits...)
		l.mu.Unlock()
		l.notifyActivityTransition()
		return
	}
	l.notifyAbnormalUpdate()
	runHooks(exits)
	l.notifyActivityTransition()
}

// catchUp runs, in time order, the ticks of the restored effects that came
// due before now: the ticks of the loading screen, which the restore instant
// put behind the replay. It always runs the earliest due tick next, the
// first restored effect first on a tie. Each runs as a scheduled tick does
// (tickAt): it counts down, and the action of an in-use effect runs. When
// the count runs out or an action reports false, the effect leaves the
// list. That removal, and any its action makes, is silent and leaves the
// icon refresh to the caller, which sends one for the whole replay; it
// returns the exit hooks that removal queued, to run after that refresh as
// Remove runs them. With no tick due it changes nothing.
func (l *List) catchUp(restored []*Effect) []func() {
	now := l.now()
	var exits []func()
	for {
		e := l.nextDue(restored, now)
		if e == nil {
			return exits
		}
		exits = append(exits, l.catchUpTick(e, now)...)
	}
}

// nextDue is the held effect among restored whose next tick is the earliest
// due at now, nil when none is due.
func (l *List) nextDue(restored []*Effect, now time.Time) *Effect {
	var next *Effect
	var nextAt time.Time
	for _, e := range restored {
		at := e.dueAt()
		if at.IsZero() || now.Before(at) || (next != nil && !at.Before(nextAt)) {
			continue
		}
		l.mu.Lock()
		held := l.holdsLocked(e)
		l.mu.Unlock()
		if held {
			next, nextAt = e, at
		}
	}
	return next
}

// catchUpTick runs e's tick due at now (see catchUp) and returns the exit
// hooks of the removal it makes.
func (l *List) catchUpTick(e *Effect, now time.Time) []func() {
	l.catchingUp.Add(1)
	defer l.catchingUp.Add(-1)
	run, remove := e.claimAction(now)
	if run && e.InUse() && !e.ActionTime() {
		remove = true
	}
	if !remove {
		return nil
	}
	var pending, exits []func()
	l.mu.Lock()
	if l.holdsLocked(e) {
		l.remove(e, &pending, &exits)
	}
	l.mu.Unlock()
	runHooks(pending)
	return exits
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

// StopAll removes every active effect, running each exit hook. The holder
// does not announce the stat change of each removal (see ModOwner.Stripped);
// a player or summon caller refreshes its view once the strip ends, an NPC
// caller sends nothing.
func (l *List) StopAll() {
	if l == nil {
		return
	}
	for _, e := range l.All() {
		l.strip(e)
	}
}

// StopAllExceptThoseThatLastThroughDeath removes active effects whose owning
// skill is not configured to persist through death, leaving their stat
// changes unannounced as StopAll does.
func (l *List) StopAllExceptThoseThatLastThroughDeath() {
	if l == nil {
		return
	}
	for _, e := range l.All() {
		if !e.Skill.StayAfterDeath {
			l.strip(e)
		}
	}
}

// strip ends e as part of a stop-all: e is marked before its removal, so a
// stack member promoted in its place still reports its own activation and
// only e's stat removal goes unannounced.
func (l *List) strip(e *Effect) {
	e.strippedAll.Store(true)
	l.Remove(e)
}

// notifyAbnormalUpdate tells l's owner to refresh its abnormal-effect icon
// state, mirroring Creature.addEffect()/removeEffect() unconditionally
// queueing an EffectList icon update on every add or remove attempt,
// regardless of whether the attempt actually changed anything.
func (l *List) notifyAbnormalUpdate() {
	if l.owner != nil && l.catchingUp.Load() == 0 {
		l.owner.UpdateEffectIcons()
	}
}

// notifyExpiry queues e's worn-off/disappeared/aborted system message: only
// for an effect that
// actually left the buffs/debuffs list (not one rejected before insertion)
// and whose template shows an icon. wornOff must be read before
// e.stopSchedule() runs, since that call zeroes e's remaining-tick counter.
// e.Skill.Toggle wins over the count check even though a toggle's schedule
// never reaches count 0: the toggle check comes first.
func (l *List) notifyExpiry(e *Effect, wornOff bool, pending *[]func()) {
	if !e.Template.Icon || l.quietLocked() {
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
// runs (still under l.mu) on failure. The owner reports the stat change
// after l.mu is released again. With announce set, a successful
// activation of an icon effect then tells the owner it feels e's effect —
// the add path's stack-head promotion does this (unless l.silent, read here
// under l.mu), the removal path's does not.
//
// It does not (re)start e's tick schedule: that starts once, in add, when e
// is first created, unconditionally for every created effect — so a
// promoted stack loser resumes with whatever count it drained down to while
// displaced instead of restarting from the template.
func (l *List) beginActivate(e *Effect, onReject func(*Effect), announce bool) func() {
	announce = announce && !l.quietLocked()
	return func() {
		ok := true
		if e.OnStart != nil {
			ok = e.OnStart(e)
		}

		attached := false
		l.mu.Lock()
		if ok {
			e.inUse = true
			if !e.startRefused {
				l.attachStatFuncs(e)
				attached = true
			}
		} else {
			onReject(e)
		}
		l.mu.Unlock()

		// The stat refresh runs outside l.mu: a RUN_SPEED change rebuilds the
		// owner's appearance for its observers, which reads this list's flags.
		if attached && l.owner != nil {
			l.owner.StatFuncsAttached(e.Funcs)
		}
		if ok && announce && !e.startRefused && e.Template.Icon && l.owner != nil {
			l.owner.NotifyEffectFelt(e.Skill.ID, e.Skill.Level)
		}
	}
}

// add inserts e, starting its tick schedule unconditionally first — every
// created effect's periodic task is scheduled up front, before any
// stacking/activation outcome is known. A RejectsIfAffected
// effect that finds its own Flag bit
// already set by any currently held effect is dropped outright before any
// buff/debuff handling: its stop-task hook fires and it never reaches the
// identical-effect replace/reject logic below, so a same-skill-id recast
// while the flag is already active is rejected here rather than treated as
// a replacement.
//
// A buff e replaces (identical) or evicts (buff-slot cap) is retired in two
// steps. Its stop-task and exit hooks are queued at once, but it stays held
// — counted, stacked, positioned and in use — for the rest of e's
// insertion, and its list removal (stat removal, next-member promotion,
// expiry message) is queued only after e's own activation. The owner
// therefore sees the old exit hook, the newcomer's start, then the old
// removal and its message, then the icon refresh. Because the old buff stays
// in use, every ending pass the insertion makes over it runs its exit hook
// again: once when retired, again if the cap eviction reaches it, and again
// when it loses its stack group's head to e.
func (l *List) add(e *Effect, pending *[]func()) {
	e.startSchedule(l.now())

	if e.RejectsIfAffected && l.flagsLocked()&e.Flag != 0 {
		appendThunk(pending, e.stopTaskThunk())
		return
	}

	var retiring []*Effect
	l.insert(e, pending, &retiring)
	for _, old := range retiring {
		// Retirement left old in use (see retireExit). It has run every exit
		// hook this insertion owes it, so its removal must not run another.
		old.inUse = false
		l.holdExit(old)
		l.remove(old, pending, pending)
	}
}

// insert places e in its visible list and stack group, retiring the buffs it
// replaces or evicts into retiring (see add).
func (l *List) insert(e *Effect, pending *[]func(), retiring *[]*Effect) {
	l.held = true
	if e.Skill.Debuff {
		for _, existing := range l.debuffs {
			if existing.identical(e) {
				appendThunk(pending, e.stopTaskThunk())
				return
			}
		}
		l.debuffs = append(l.debuffs, e)
		l.publishFlagsLocked()
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
// identical check already retired) queues the exit hook again, but no second
// stop-task hook: e's schedule is already stopped.
func retire(e *Effect, pending *[]func(), retiring *[]*Effect) {
	if !slices.Contains(*retiring, e) {
		appendThunk(pending, e.stopTaskThunk())
		*retiring = append(*retiring, e)
	}
	appendThunk(pending, e.retireExit())
}
