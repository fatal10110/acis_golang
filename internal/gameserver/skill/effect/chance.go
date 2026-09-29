package effect

import (
	"slices"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// AddChanceTrigger registers e, a started chance-skill-trigger effect, as
// one of the owner's chance procs. Registering the same effect twice keeps
// one entry.
func (l *List) AddChanceTrigger(e *Effect) {
	if l == nil || e == nil {
		return
	}
	l.triggersMu.Lock()
	defer l.triggersMu.Unlock()
	if !slices.Contains(l.triggers, e) {
		l.triggers = append(l.triggers, e)
	}
}

// RemoveChanceTrigger drops e from the owner's chance procs.
func (l *List) RemoveChanceTrigger(e *Effect) {
	if l == nil || e == nil {
		return
	}
	l.triggersMu.Lock()
	defer l.triggersMu.Unlock()
	l.triggers = slices.DeleteFunc(l.triggers, func(t *Effect) bool { return t == e })
}

// ChanceTriggers returns a snapshot of the owner's registered chance procs,
// in registration order.
func (l *List) ChanceTriggers() []*Effect {
	if l == nil {
		return nil
	}
	l.triggersMu.Lock()
	defer l.triggersMu.Unlock()
	return slices.Clone(l.triggers)
}

// ChanceCondition is the activation rule of a chance-skill-trigger effect:
// the event it reacts to and its activation roll. ok is false for an effect
// whose template names no trigger event.
func (e *Effect) ChanceCondition() (cond modelskill.ChanceCondition, ok bool) {
	cond, ok, err := modelskill.ParseChanceCondition(e.Template.ChanceType, e.Template.ActivationChance)
	return cond, ok && err == nil
}
