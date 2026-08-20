package effect

import (
	"math"
	"slices"
)

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
	e.stopSchedule()

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

// remove drops e from the list for good — the only place e's tick schedule
// stops, mirroring stopEffectTask() firing once scheduleEffect() reaches
// FINISHING (AbstractEffect.java:308-320). Mere stacking displacement (see
// addStacked) does not call remove: a displaced member stays queued and its
// schedule keeps draining.
func (l *List) remove(e *Effect, pending *[]func()) {
	e.stopSchedule()

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
		l.owner.RemoveStatsByOwner(ModOwnerEffect(e))
	}
}
