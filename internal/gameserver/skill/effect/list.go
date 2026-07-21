package effect

import (
	"math"
	"slices"
	"sync"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
)

// Type classifies the runtime behavior family of an effect.
type Type string

const (
	// TypeBuff is a beneficial persistent effect.
	TypeBuff Type = "BUFF"
	// TypeDebuff is a harmful persistent effect.
	TypeDebuff Type = "DEBUFF"
	// TypeDamOverTime is a periodic HP damage effect.
	TypeDamOverTime Type = "DMG_OVER_TIME"
	// TypeFear is a forced flee disabler.
	TypeFear Type = "FEAR"
	// TypeRoot is a movement disabler.
	TypeRoot Type = "ROOT"
	// TypeSleep is an action disabler.
	TypeSleep Type = "SLEEP"
	// TypeStun is an attack, cast, and movement disabler.
	TypeStun Type = "STUN"
)

// Skill carries the skill fields the effect container needs for ordering and
// duplicate handling, plus the classification tags a disabling/dispelling
// skill needs to recognize an already-active effect as "the same kind" of
// disable or "eligible to be stripped".
type Skill struct {
	ID modelskill.ID
	// Level is the applied level of the skill that owns this effect
	// instance's template.
	Level int
	// SkillType is the raw datapack skill-type tag (e.g. "BUFF", "REFLECT").
	// It drives the buff-slot family used by the list's cap enforcement.
	SkillType      string
	Debuff         bool
	Toggle         bool
	KillByDOT      bool
	CanBeDispelled bool

	// MagicLevel is the owning skill's casting level, read by cancel-family
	// effects to compare a caster's cancel power against each candidate
	// effect's own owning-skill level.
	MagicLevel int
	// AbnormalLevel and EffectAbnormalLevel are the owning skill's cancel-
	// threshold tags: EffectAbnormalLevel applies when EffectType is set,
	// AbnormalLevel otherwise. A negate-family effect strips a candidate
	// only when the candidate's applicable level is within its threshold.
	AbnormalLevel       int
	EffectAbnormalLevel int
	// EffectType is the owning skill's classification tag (distinct from
	// the per-effect-template tag exposed by Effect.ClassTag), consulted
	// alongside SkillType when a negate-family effect matches a candidate
	// by classification.
	EffectType string

	// MaxNegatedEffects caps how many candidates a cancel-family effect
	// strips in one activation; 0 means unlimited.
	MaxNegatedEffects int
	// NegateLevel, NegateIDs, and NegateTypes configure a negate-family
	// effect: NegateIDs strips candidates by their owning skill id,
	// NegateTypes strips candidates by classification (gated by
	// NegateLevel, or ungated when -1).
	NegateLevel int
	NegateIDs   []int
	NegateTypes []string
}

func (s Skill) sevenSigns() bool {
	return s.ID > 4360 && s.ID < 4367
}

// buffSlotFamily is the set of skill types that occupy (and can be evicted
// from) an owner's limited buff slots.
var buffSlotFamily = map[string]bool{
	"BUFF":             true,
	"REFLECT":          true,
	"HEAL_PERCENT":     true,
	"HEAL_STATIC":      true,
	"MANAHEAL_PERCENT": true,
	"COMBATPOINTHEAL":  true,
}

func (s Skill) buffSlot() bool {
	return buffSlotFamily[s.SkillType]
}

// Effect is one live skill effect managed by a List. Hook fields are optional;
// absent hooks behave as a successful no-op.
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

	inUse bool
}

// InUse reports whether e is the active member of its stack group.
func (e *Effect) InUse() bool {
	if e == nil {
		return false
	}
	return e.inUse
}

// ActionTime runs e's periodic hook. Effects without periodic behavior stop
// after one action tick.
func (e *Effect) ActionTime() bool {
	if e == nil || e.OnAction == nil {
		return false
	}
	return e.OnAction(e)
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

// StatOwner receives stat function changes when active effects change and
// reports the owner's current buff-slot capacity.
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
// runs (still under l.mu) on failure.
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

// add inserts e. A RejectsIfAffected effect that finds its own Flag bit
// already set by any currently held effect is dropped outright before any
// buff/debuff handling: its stop-task hook fires and it never reaches the
// identical-effect replace/reject logic below, so a same-skill-id recast
// while the flag is already active is rejected here rather than treated as
// a replacement.
func (l *List) add(e *Effect, pending *[]func()) {
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
func (l *List) doesStack(e *Effect) bool {
	stackType := e.stackType()
	if stackType == "none" {
		return false
	}
	for _, existing := range l.stacks[stackType] {
		if existing != nil && !existing.Skill.Debuff {
			return true
		}
	}
	return false
}

// buffCount returns the number of visible, non-seven-signs buff-slot-family
// buffs currently held.
func (l *List) buffCount() int {
	count := 0
	for _, e := range l.buffs {
		if e != nil && e.Template.Icon && !e.Skill.sevenSigns() && e.Skill.buffSlot() {
			count++
		}
	}
	return count
}

// maxBuffCount is the owner's buff-slot capacity, or unbounded when no
// owner is set.
func (l *List) maxBuffCount() int {
	if l.owner == nil {
		return math.MaxInt
	}
	return l.owner.MaxBuffCount()
}

// evictForCap exits the oldest buff-slot-family buffs when e would put the
// list at or over the owner's buff-slot cap. Only buff-slot-family incoming
// effects trigger eviction, and only buff-slot-family buffs are evicted.
func (l *List) evictForCap(e *Effect, pending *[]func()) {
	if !e.Skill.buffSlot() {
		return
	}
	remaining := l.buffCount() - l.maxBuffCount()
	if remaining < 0 {
		return
	}
	for _, existing := range slices.Clone(l.buffs) {
		if existing == nil || !existing.Skill.buffSlot() {
			continue
		}
		l.exit(existing, pending)
		remaining--
		if remaining < 0 {
			break
		}
	}
}

func (l *List) insertBuff(e *Effect) {
	if e.Skill.Toggle {
		l.buffs = append(l.buffs, e)
		return
	}

	pos := 0
	for _, existing := range l.buffs {
		if existing == nil || existing.Skill.Toggle || existing.Skill.sevenSigns() {
			continue
		}
		pos++
	}
	l.buffs = slices.Insert(l.buffs, pos, e)
}

func (l *List) addStacked(e *Effect, pending *[]func()) {
	if l.stacks == nil {
		l.stacks = make(map[string][]*Effect)
	}

	stackType := e.stackType()
	queue := l.stacks[stackType]
	var deactivate *Effect
	if len(queue) > 0 {
		deactivate = l.contained(queue[0])
		pos := 0
		for pos < len(queue) && e.Template.StackOrder < queue[pos].Template.StackOrder {
			pos++
		}
		queue = slices.Insert(queue, pos, e)
		if l.cancelLesser && !e.Herb && len(queue) > 1 {
			victim := queue[1]
			queue = slices.Delete(queue, 1, 2)
			l.removeFromVisible(victim)
		}
	} else {
		queue = append(queue, e)
	}
	l.stacks[stackType] = queue

	activate := l.contained(queue[0])
	if deactivate == activate {
		return
	}

	if deactivate != nil {
		*pending = append(*pending, func() { l.removeStats(deactivate) })
		appendThunk(pending, deactivate.beginExit())
	}
	if activate != nil {
		*pending = append(*pending, l.beginActivate(activate, func(rejected *Effect) { l.removeRejectedStacked(rejected) }))
	}
}

func (l *List) removeRejectedStacked(e *Effect) {
	stackType := e.stackType()
	queue := l.stacks[stackType]
	if index := slices.Index(queue, e); index >= 0 {
		queue = slices.Delete(queue, index, index+1)
	}
	if len(queue) == 0 {
		delete(l.stacks, stackType)
	} else {
		l.stacks[stackType] = queue
	}
	l.removeFromVisible(e)
}

func (l *List) remove(e *Effect, pending *[]func()) {
	if e.stackType() == "none" {
		if l.removeFromVisible(e) && e.InUse() {
			*pending = append(*pending, func() { l.removeStats(e) })
			appendThunk(pending, e.beginExit())
		}
		return
	}

	queue := l.stacks[e.stackType()]
	index := slices.Index(queue, e)
	if index < 0 {
		l.removeFromVisible(e)
		return
	}

	queue = slices.Delete(queue, index, index+1)
	if index == 0 {
		*pending = append(*pending, func() { l.removeStats(e) })
		appendThunk(pending, e.beginExit())
		if len(queue) > 0 {
			next := l.contained(queue[0])
			if next != nil {
				*pending = append(*pending, l.beginActivate(next, func(*Effect) {}))
			}
		}
	}

	if len(queue) == 0 {
		delete(l.stacks, e.stackType())
	} else {
		l.stacks[e.stackType()] = queue
	}
	l.removeFromVisible(e)
}

func (l *List) contained(e *Effect) *Effect {
	if slices.Contains(l.buffs, e) || slices.Contains(l.debuffs, e) {
		return e
	}
	return nil
}

func (l *List) removeFromVisible(e *Effect) bool {
	if e.Skill.Debuff {
		return removeEffect(&l.debuffs, e)
	}
	return removeEffect(&l.buffs, e)
}

func removeEffect(effects *[]*Effect, e *Effect) bool {
	index := slices.Index(*effects, e)
	if index < 0 {
		return false
	}
	*effects = slices.Delete(*effects, index, index+1)
	return true
}

func (l *List) addStatFuncs(e *Effect) {
	if l.owner != nil {
		l.owner.AddStatFuncs(e.Funcs)
	}
}

func (l *List) removeStats(e *Effect) {
	if l.owner != nil {
		l.owner.RemoveStatsByOwner(e)
	}
}
