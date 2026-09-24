package effect

import (
	"sync"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// StatOwner is what a List is attached to: the holder its stat modifiers
// land on, the client icons its changes refresh and the recipient of its
// expiry messages. Kinds without client icons or messages implement those as
// no-ops.
type StatOwner interface {
	AddStatFuncs([]Mod)
	RemoveStatsByOwner(owner ModOwner)
	// MaxBuffCount is the number of non-toggle, non-seven-signs buffs the
	// owner can hold at once (base slot count plus any bonus the owner
	// grants, e.g. from a known passive).
	MaxBuffCount() int

	// UpdateEffectIcons refreshes the owner's effect icons after an add or
	// remove attempt.
	UpdateEffectIcons()
	// NotifyEffectWornOff, NotifyEffectDisappeared and NotifyEffectAborted
	// send the system message that accompanies an icon effect leaving the
	// list.
	NotifyEffectWornOff(skillID modelskill.ID, level int)
	NotifyEffectDisappeared(skillID modelskill.ID, level int)
	NotifyEffectAborted(skillID modelskill.ID, level int)
}

// Option changes List behavior.
type Option func(*List)

// cancelLesserEnabled is the process-wide CancelLesserEffect switch
// (default true). The composition root sets this once at boot from
// players.properties.
var cancelLesserEnabled = true

// SetCancelLesser records the process-wide CancelLesserEffect switch.
func SetCancelLesser(enabled bool) { cancelLesserEnabled = enabled }

// CancelLesser reports whether a newly stacked non-herb effect removes the
// lower-priority effect it displaces.
func CancelLesser() bool { return cancelLesserEnabled }

// WithCancelLesser overrides the process-wide CancelLesserEffect switch for
// one list. Tests use this to pin a list independent of boot config.
func WithCancelLesser(cancel bool) Option {
	return func(l *List) {
		l.cancelLesser = cancel
		l.cancelLesserSet = true
	}
}

// WithClock makes a list that runs on no queue of its own read the time
// from q's clock, so its effect periods follow the same clock as the timers
// of q's owner.
func WithClock(q *sim.Queue) Option {
	return func(l *List) { l.clock = q }
}

// ActivityRegistry records whether a list has effects to tick.
type ActivityRegistry interface {
	SetActive(*List, bool)
}

// WithActivityRegistry associates a list with one server's effect ticker.
func WithActivityRegistry(registry ActivityRegistry) Option {
	return func(l *List) { l.activity = registry }
}

// Untrack unconditionally deregisters l from the process-wide activity
// registry, regardless of whether it currently holds an effect. Call it
// when l's owner leaves the world for good (logout, NPC decay, unsummon,
// signet expiry) so a list that still holds an effect doesn't keep ticking
// a detached actor forever. This mirrors the pre-registry behavior, where
// an actor leaving world.State silently dropped out of the tick scan: it
// does not run any effect's exit hook or otherwise touch buffs/debuffs,
// only stops future Tick calls from reaching this list.
//
// Like notifyActivityTransition, it decides and applies under one hold of
// l.mu so it can't race a concurrent Add/Remove on the same list into
// re-registering it right after Untrack deregisters it, or vice versa.
func (l *List) Untrack() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.tracked {
		return
	}
	l.tracked = false
	if l.activity != nil {
		l.activity.SetActive(l, false)
	}
}

// emptyLocked reports whether l currently holds no buff or debuff. Caller
// must hold l.mu.
func (l *List) emptyLocked() bool {
	return len(l.buffs) == 0 && len(l.debuffs) == 0
}

// List owns one creature's active buffs and debuffs. All methods are safe for
// concurrent use; mu guards buffs, debuffs, stacks, tracked, and callbacks
// into owner.
type List struct {
	mu sync.Mutex

	owner           StatOwner
	activity        ActivityRegistry
	cancelLesser    bool
	cancelLesserSet bool

	buffs   []*Effect
	debuffs []*Effect
	stacks  map[string][]*Effect

	// tracked records whether l is currently registered with the
	// activity registry, so notifyActivityTransition can
	// reconcile against l's own last-known state instead of a value a
	// caller captured before releasing mu — see notifyActivityTransition.
	tracked bool

	// queue is the owner's queue, which periodic effect actions run on; set
	// once before the owner is published.
	queue *sim.Queue
	// clock is what effect periods are measured on: the owner's queue, else
	// the WithClock queue, else the wall clock.
	clock sim.Clock
}

func (l *List) now() time.Time { return l.clock.Now() }

// SetQueue makes q, the owner's queue, the queue this list's periodic
// actions run on and the clock its effect periods are measured on.
func (l *List) SetQueue(q *sim.Queue) {
	l.queue = q
	l.clock = q
}

// Queue returns the queue SetQueue installed, or nil.
func (l *List) Queue() *sim.Queue { return l.queue }

// NewList returns an empty effect list.
func NewList(owner StatOwner, opts ...Option) *List {
	// ponytail: wall-clock default serves only queue-less unit fixtures; #2488 drops it.
	l := &List{owner: owner, clock: sim.SystemClock{}}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Flags returns the union of every visible held effect's flag bits — the
// active member of each stack group. Stacked-but-inactive members live only
// in the stacks map and contribute nothing, unlike the reference, whose
// computeEffectFlags() ORs every entry in _buffs/_debuffs including
// stacked-but-not-in-use ones (EffectList.java:896-921). The difference is
// unreachable for the crowd-control flags that consume this surface: the
// kinds carrying them either reject same-flag re-application
// (rejectsIfAffected) or activate the next stack member the instant the
// active one leaves, so the union stays continuous either way. It is
// recomputed from the current buffs and debuffs on every call rather than
// cached, matching how rarely a caller needs it compared to how often the
// list itself changes.
func (l *List) Flags() Flag {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.flagsLocked()
}

// flagsLocked is Flags' body for callers that already hold l.mu (e.g. add,
// which cannot call the exported Flags/IsAffected without self-deadlocking
// on the non-reentrant mutex).
func (l *List) flagsLocked() Flag {
	var flags Flag
	for _, e := range l.buffs {
		if e != nil {
			flags |= e.Flag
		}
	}
	for _, e := range l.debuffs {
		if e != nil {
			flags |= e.Flag
		}
	}
	return flags
}

// IsAffected reports whether any bit of flag is set in l.Flags().
func (l *List) IsAffected(flag Flag) bool {
	return l.Flags()&flag != 0
}

// AIDenyFlags are the effect flags that keep an actor from taking AI actions.
const AIDenyFlags = FlagStunned | FlagMeditating | FlagSleep | FlagParalyzed | FlagFear

// StartedAffected is IsAffected limited to effects whose on-start hook has
// completed. Called from inside an on-start hook, it answers for the state
// the actor was in before that effect landed: a hook must not see its own
// flag when deciding how an actor that was already disabled reacts.
func (l *List) StartedAffected(flag Flag) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, group := range [][]*Effect{l.buffs, l.debuffs} {
		for _, e := range group {
			if e != nil && e.inUse && e.Flag&flag != 0 {
				return true
			}
		}
	}
	return false
}

func (l *List) shouldCancelLesser() bool {
	if l.cancelLesserSet {
		return l.cancelLesser
	}
	return cancelLesserEnabled
}

// All returns a snapshot of effects ordered as buffs followed by debuffs.
func (l *List) All() []*Effect {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	effects := make([]*Effect, 0, len(l.buffs)+len(l.debuffs))
	effects = append(effects, l.buffs...)
	effects = append(effects, l.debuffs...)
	return effects
}

func (l *List) active() []*Effect {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	effects := make([]*Effect, 0, len(l.buffs)+len(l.debuffs))
	for _, e := range l.buffs {
		if e != nil && e.inUse {
			effects = append(effects, e)
		}
	}
	for _, e := range l.debuffs {
		if e != nil && e.inUse {
			effects = append(effects, e)
		}
	}
	return effects
}

// ActiveBySkillID returns the applied Level of the first currently active
// effect owned by skill id, and whether one was found. This is the
// getFirstEffect(skillId) lookup ConditionElementSeed/ConditionForceBuff-
// style consumers need for live seed-charge power and Force-buff level.
func (l *List) ActiveBySkillID(id int) (level int, ok bool) {
	for _, e := range l.active() {
		if int(e.Skill.ID) == id {
			return e.Level, true
		}
	}
	return 0, false
}

// DanceCount returns the number of active dance/song effects, mirroring
// Java's EffectList.getDanceCount(). It drives the per-cast MP surcharge a
// dance/song skill pays for each already-running dance/song: casting
// another one gets more expensive as more stay active simultaneously.
func (l *List) DanceCount() int {
	count := 0
	for _, e := range l.active() {
		if e.Skill.Dance {
			count++
		}
	}
	return count
}

// Tick runs periodic actions due at the current time and removes effects
// whose action hook stops or whose configured tick count is exhausted.
