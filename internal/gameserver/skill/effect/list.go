package effect

import (
	"sync"
	"sync/atomic"
)

type StatOwner interface {
	AddStatFuncs([]Mod)
	RemoveStatsByOwner(owner ModOwner)
	// MaxBuffCount is the number of non-toggle, non-seven-signs buffs the
	// owner can hold at once (base slot count plus any bonus the owner
	// grants, e.g. from a known passive).
	MaxBuffCount() int
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

// activityHook is the process-wide registrar of lists that currently hold at
// least one effect, wired once at boot (task.Effects) before any List is
// constructed. It lets the periodic effect tick iterate only lists with
// something to tick instead of scanning every tracked world object every
// second. A nil hook (tests, tools that never call SetActivityHook) leaves
// Add/Remove exactly as before.
//
// Stored behind an atomic.Pointer because Add/Remove/Untrack read it from
// every goroutine that applies an effect, concurrently with SetActivityHook
// being called from whichever goroutine boots the process (or, in tests,
// boots a server).
var activityHook atomic.Pointer[func(list *List, active bool)]

// SetActivityHook installs the process-wide list-activity registrar.
func SetActivityHook(hook func(list *List, active bool)) {
	activityHook.Store(&hook)
}

// callActivityHook invokes the installed activity hook, if any.
func callActivityHook(list *List, active bool) {
	if hook := activityHook.Load(); hook != nil && *hook != nil {
		(*hook)(list, active)
	}
}

// Untrack unconditionally deregisters l from the process-wide activity
// registry, regardless of whether it currently holds an effect. Call it
// when l's owner leaves the world for good (logout, NPC decay, unsummon,
// signet expiry) so a list that still holds an effect doesn't keep ticking
// a detached actor forever. This mirrors the pre-registry behavior, where
// an actor leaving world.State silently dropped out of the tick scan: it
// does not run any effect's exit hook or otherwise touch buffs/debuffs,
// only stops future Tick calls from reaching this list.
func (l *List) Untrack() {
	if l == nil {
		return
	}
	callActivityHook(l, false)
}

// emptyLocked reports whether l currently holds no buff or debuff. Caller
// must hold l.mu.
func (l *List) emptyLocked() bool {
	return len(l.buffs) == 0 && len(l.debuffs) == 0
}

// List owns one creature's active buffs and debuffs. All methods are safe for
// concurrent use; mu guards buffs, debuffs, stacks, and callbacks into owner.
type List struct {
	mu sync.Mutex

	owner           StatOwner
	cancelLesser    bool
	cancelLesserSet bool

	buffs   []*Effect
	debuffs []*Effect
	stacks  map[string][]*Effect
}

// NewList returns an empty effect list.
func NewList(owner StatOwner, opts ...Option) *List {
	l := &List{owner: owner}
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
