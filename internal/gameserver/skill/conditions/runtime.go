package conditions

import (
	"strconv"
	"strings"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// EvaluateSkill reports whether every condition clause on def accepts the
// supplied caster and target. Conditions are data-driven, so an unknown
// predicate rejects instead of silently bypassing a restriction.
func EvaluateSkill(def modelskill.Definition, caster, target any) bool {
	for _, clause := range def.Conditions {
		if !evaluate(clause.Root, caster, target) {
			return false
		}
	}
	return true
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
