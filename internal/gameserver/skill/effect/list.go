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
	// SkillType is the raw datapack skill-type tag (e.g. "BUFF", "REFLECT").
	// It drives the buff-slot family used by the list's cap enforcement.
	SkillType      string
	Debuff         bool
	Toggle         bool
	KillByDOT      bool
	CanBeDispelled bool
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

func (e *Effect) setInUse(inUse bool) bool {
	if inUse {
		if e.OnStart != nil && !e.OnStart(e) {
			return false
		}
		e.inUse = true
		return true
	}
	if !e.inUse {
		return true
	}
	e.inUse = false
	if e.OnExit != nil {
		e.OnExit(e)
	}
	return true
}

func (e *Effect) stopTask() {
	if e.OnStopTask != nil {
		e.OnStopTask(e)
	}
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
	l.mu.Lock()
	defer l.mu.Unlock()

	l.add(e)
}

// Remove drops e from the list and activates the next member of its stack
// group when one exists.
func (l *List) Remove(e *Effect) {
	if l == nil || e == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.remove(e)
}

// add inserts e. Incoming effects are not yet gated by the owner's active
// special-effect state (e.g. rejecting an effect that conflicts with a
// currently active status) — that needs an owner-side abnormal-state tracker
// that doesn't exist yet; wire this gate in once that tracker lands.
func (l *List) add(e *Effect) {
	if e.Skill.Debuff {
		for _, existing := range l.debuffs {
			if existing.identical(e) {
				e.stopTask()
				return
			}
		}
		l.debuffs = append(l.debuffs, e)
	} else {
		for _, existing := range slices.Clone(l.buffs) {
			if existing.identical(e) {
				l.exit(existing)
			}
		}

		// Herbs never evict a real buff: at or over capacity, they are
		// simply dropped.
		if e.Herb && l.buffCount() >= l.maxBuffCount() {
			e.stopTask()
			return
		}

		if !l.doesStack(e) && !e.Skill.sevenSigns() {
			l.evictForCap(e)
		}

		l.insertBuff(e)
	}

	if e.stackType() == "none" {
		if e.setInUse(true) {
			l.addStatFuncs(e)
		} else {
			l.removeFromVisible(e)
		}
		return
	}

	l.addStacked(e)
}

// exit fully retires e: its scheduled task is stopped and it is detached
// from stats/visibility and, if active, run through its on-exit hook.
func (l *List) exit(e *Effect) {
	e.stopTask()
	l.remove(e)
}

// doesStack reports whether e's stack type already has a member among the
// current buffs, mirroring the check that exempts stacking buffs from
// buff-slot cap eviction.
func (l *List) doesStack(e *Effect) bool {
	stackType := e.stackType()
	if stackType == "none" {
		return false
	}
	return len(l.stacks[stackType]) > 0
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
func (l *List) evictForCap(e *Effect) {
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
		l.exit(existing)
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

func (l *List) addStacked(e *Effect) {
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
		l.removeStats(deactivate)
		deactivate.setInUse(false)
	}
	if activate != nil {
		if activate.setInUse(true) {
			l.addStatFuncs(activate)
		} else {
			l.removeRejectedStacked(activate)
		}
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

func (l *List) remove(e *Effect) {
	if e.stackType() == "none" {
		if l.removeFromVisible(e) && e.InUse() {
			l.removeStats(e)
			e.setInUse(false)
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
		l.removeStats(e)
		e.setInUse(false)
		if len(queue) > 0 {
			next := l.contained(queue[0])
			if next != nil && next.setInUse(true) {
				l.addStatFuncs(next)
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
