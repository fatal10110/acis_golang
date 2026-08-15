package conditions

import (
	"strconv"
	"strings"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// caster/target here stay any: this is the separate data-driven
// modelskill.Condition interpreter (<cond> XML clauses), not this package's
// typed Condition contract. No production type implements conditions.Actor
// yet, so this path can't be typed against it until #1087 lands; that's
// #885's job.

// EvaluateSkill reports the first condition clause that rejects the supplied
// caster and target. The returned clause carries the feedback configured by
// its <cond> element.
func EvaluateSkill(def modelskill.Definition, caster, target any) (modelskill.ConditionClause, bool) {
	for _, clause := range def.Conditions {
		if !evaluate(clause.Root, caster, target) {
			return clause, false
		}
	}
	return modelskill.ConditionClause{}, true
}

func evaluate(cond modelskill.Condition, caster, target any) bool {
	switch cond.Kind {
	case "and":
		for _, child := range cond.Children {
			if !evaluate(child, caster, target) {
				return false
			}
		}
		return true
	case "or":
		for _, child := range cond.Children {
			if evaluate(child, caster, target) {
				return true
			}
		}
		return false
	case "not":
		return len(cond.Children) == 1 && !evaluate(cond.Children[0], caster, target)
	case "player":
		return evaluatePlayer(cond.Attrs, caster)
	default:
		return false
	}
}

func evaluatePlayer(attrs map[string]string, caster any) bool {
	for name, raw := range attrs {
		want, err := strconv.ParseBool(strings.ToLower(raw))
		if err != nil {
			return false
		}
		switch strings.ToLower(name) {
		case "flying":
			flying := false
			if state, ok := caster.(interface{ IsFlying() bool }); ok {
				flying = state.IsFlying()
			} else if state, ok := caster.(interface{ Flying() bool }); ok {
				flying = state.Flying()
			}
			if flying != want {
				return false
			}
		default:
			return false
		}
	}
	return true
}
