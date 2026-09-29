package conditions

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Source is a live creature that can present itself to a condition test.
type Source interface {
	ConditionActor() Actor
}

// ActorOf returns v's condition view, or nil when v is nil or is not a
// creature (a ground item, a static object, no selection at all).
func ActorOf(v any) Actor {
	s, ok := v.(Source)
	if !ok || s == nil {
		return nil
	}
	return s.ConditionActor()
}

// EvaluateSkill reports the first of def's <cond> clauses that rejects the
// caster/target pair, with the feedback that clause carries. target is
// whatever the caster has selected (see ActorOf); a nil or non-creature
// target fails every target-side test. A clause that does not compile, or a
// nil caster, rejects rather than letting the cast through.
func EvaluateSkill(def modelskill.Definition, caster Source, target any) (modelskill.ConditionClause, bool) {
	if len(def.Conditions) == 0 {
		return modelskill.ConditionClause{}, true
	}
	effector := ActorOf(caster)
	effected := ActorOf(target)
	for _, clause := range def.Conditions {
		cond, err := Compile(clause.Root)
		if err != nil || effector == nil || !cond.Test(effector, effected, nil) {
			return clause, false
		}
	}
	return modelskill.ConditionClause{}, true
}
