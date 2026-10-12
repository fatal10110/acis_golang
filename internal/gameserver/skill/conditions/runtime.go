package conditions

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
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
// caster/target pair, with the feedback that clause carries. def is the
// skill under test (see SkillStats). target is
// whatever the caster has selected (see ActorOf); a nil or non-creature
// target fails every target-side test. A clause that does not compile, or a
// nil caster, rejects rather than letting the cast through. A clause whose
// test aborts (its root holds no condition, or the test reached a <not>
// around one, see Evaluate) rejects with no feedback: only its Root is
// reported.
func EvaluateSkill(def modelskill.Definition, caster Source, target any) (modelskill.ConditionClause, bool) {
	if len(def.Conditions) == 0 {
		return modelskill.ConditionClause{}, true
	}
	effector := ActorOf(caster)
	effected := ActorOf(target)
	for _, clause := range def.Conditions {
		cond, err := Compile(clause.Root)
		if err != nil || effector == nil {
			return clause, false
		}
		held, aborted := Evaluate(cond, effector, effected, definitionSkill{def.Stat})
		if aborted {
			return modelskill.ConditionClause{Root: clause.Root}, false
		}
		if !held {
			return clause, false
		}
	}
	return modelskill.ConditionClause{}, true
}

// definitionSkill is a skill definition under test: its "stat" set value,
// an upper-case stat constant name, names its stat. An empty or unknown
// value names none.
type definitionSkill struct{ stat string }

func (s definitionSkill) Stat() (stat.Stat, bool) {
	st, err := stat.ByConstantName(s.stat)
	return st, err == nil
}

// EvaluateItem tests an item's use conditions against effector using the
// item on effected, with no skill under test. It reports the first clause
// that rejects the use (held false), with the feedback that clause carries.
// A clause whose root holds no condition is skipped. A clause that does not
// compile rejects with its feedback. A test that aborts (it reached a <not>
// around no condition, see Evaluate) reports aborted: the use is refused
// without that clause's feedback. A nil effector fails the first clause
// that holds a condition.
func EvaluateItem(uses []item.UseCondition, effector, effected Actor) (failed item.UseCondition, held, aborted bool) {
	for _, uc := range uses {
		cond, err := Compile(ItemCondition(uc.Root))
		if err != nil || (cond != nil && effector == nil) {
			return uc, false, false
		}
		if cond == nil {
			continue
		}
		ok, abort := Evaluate(cond, effector, effected, nil)
		if abort {
			return item.UseCondition{Root: uc.Root}, false, true
		}
		if !ok {
			return uc, false, false
		}
	}
	return item.UseCondition{}, true, false
}

// ItemCondition is c in the skill condition shape Compile reads; both carry
// the same parsed element tree.
func ItemCondition(c item.Condition) modelskill.Condition {
	out := modelskill.Condition{Kind: c.Kind, Attrs: c.Attrs}
	for _, ch := range c.Children {
		out.Children = append(out.Children, ItemCondition(ch))
	}
	return out
}
