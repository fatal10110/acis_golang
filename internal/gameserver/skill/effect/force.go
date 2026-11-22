package effect

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

// IncreaseForce grows the force effect of skill id that l holds by one level,
// up to maxLevel: it removes that effect and hands reapply the grown level.
// With no such effect held it calls start instead, and with one already at
// maxLevel it does nothing.
//
// Force and seed changes on one list run one at a time, each from its read of
// the held effect through its reapply or start, so concurrent channellers
// raise the force once each. The read and the removal share one hold of the
// list lock: the effect read is the one removed. reapply and start run with
// the list lock released, as an add takes it itself.
func (l *List) IncreaseForce(id modelskill.ID, maxLevel int, reapply func(level int), start func()) {
	if l == nil {
		return
	}
	l.forceMu.Lock()
	defer l.forceMu.Unlock()

	level, removed, held := l.removeForce(id, func(level int) bool { return level < maxLevel })
	switch {
	case !held:
		if start != nil {
			start()
		}
	case removed && reapply != nil:
		reapply(level + 1)
	}
}

// DecreaseForce shrinks the force effect of skill id that l holds by one
// level: it removes that effect and hands reapply the lowered level, unless
// that drops below 1. It runs one at a time with the other force and seed
// changes on l (see IncreaseForce).
func (l *List) DecreaseForce(id modelskill.ID, reapply func(level int)) {
	if l == nil {
		return
	}
	l.forceMu.Lock()
	defer l.forceMu.Unlock()

	level, removed, _ := l.removeForce(id, func(int) bool { return true })
	if removed && level-1 >= 1 && reapply != nil {
		reapply(level - 1)
	}
}

// removeForce removes the first effect of skill id that l holds, buffs first,
// when change accepts its level. It reports that level, whether it removed
// the effect and whether l held one at all.
func (l *List) removeForce(id modelskill.ID, change func(level int) bool) (level int, removed, held bool) {
	var pending, exits []func()
	l.mu.Lock()
	e := l.firstBySkillLocked(id)
	if e == nil {
		l.mu.Unlock()
		return 0, false, false
	}
	level = e.Level
	if !change(level) {
		l.mu.Unlock()
		return level, false, true
	}
	l.remove(e, &pending, &exits)
	l.mu.Unlock()
	l.finishRemove(pending, exits)
	return level, true, true
}

// IncreasePower grows the charge of the seed effect of skill id that l holds
// by one, in place: the same instance keeps running and its schedule is
// untouched. With no such effect held it calls start instead. It runs one at
// a time with the other force and seed changes on l (see IncreaseForce).
func (l *List) IncreasePower(id modelskill.ID, start func()) {
	if l == nil {
		return
	}
	l.forceMu.Lock()
	defer l.forceMu.Unlock()

	l.mu.Lock()
	e := l.firstBySkillLocked(id)
	if e != nil {
		e.Level++
	}
	l.mu.Unlock()
	if e == nil && start != nil {
		start()
	}
}

// Level reports the applied level of e, read under the list lock that a
// seed's charge grows under (see IncreasePower).
func (l *List) Level(e *Effect) int {
	if l == nil || e == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return e.Level
}

// firstBySkillLocked returns the first effect of skill id that l holds,
// buffs before debuffs, or nil. Caller must hold l.mu.
func (l *List) firstBySkillLocked(id modelskill.ID) *Effect {
	for _, effects := range [2][]*Effect{l.buffs, l.debuffs} {
		for _, e := range effects {
			if e != nil && e.Skill.ID == id {
				return e
			}
		}
	}
	return nil
}
