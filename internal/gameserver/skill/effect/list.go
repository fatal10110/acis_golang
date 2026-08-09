package effect

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
)

type StatOwner interface {
	AddStatFuncs([]basefunc.Func)
	RemoveStatsByOwner(owner any)
	// MaxBuffCount is the number of non-toggle, non-seven-signs buffs the
	// owner can hold at once (base slot count plus any bonus the owner
	// grants, e.g. from a known passive).
	MaxBuffCount() int
}

// Option changes List behavior.
type Option func(*List)

// WithCancelLesser controls whether a newly stacked non-herb effect removes
// the lower-priority effect it displaces. The default is true.
func WithCancelLesser(cancel bool) Option {
	return func(l *List) {
		l.cancelLesser = cancel
	}
}

// List owns one creature's active buffs and debuffs. All methods are safe for
// concurrent use; mu guards buffs, debuffs, stacks, and callbacks into owner.
type List struct {
	mu sync.Mutex

	owner        StatOwner
	cancelLesser bool

	buffs   []*Effect
	debuffs []*Effect
	stacks  map[string][]*Effect
}

// NewList returns an empty effect list.
func NewList(owner StatOwner, opts ...Option) *List {
	l := &List{
		owner:        owner,
		cancelLesser: true,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Flags returns the union of every held effect's flag bits, across both
// active and stacked-but-not-yet-active members. It is recomputed from the
// current buffs and debuffs on every call rather than cached, matching how
// rarely a caller needs it compared to how often the list itself changes.
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
