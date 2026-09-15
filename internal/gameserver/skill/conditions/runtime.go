package conditions

import (
	"strconv"
	"strings"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SkillCaster is the caster state the data-driven modelskill.Condition
// interpreter (<cond> XML clauses) reads. It is separate from this package's
// typed Condition contract.
type SkillCaster interface {
	Flying() bool
}

// EvaluateSkill reports the first condition clause that rejects the supplied
// caster and target. The returned clause carries the feedback configured by
// its <cond> element.
func EvaluateSkill(def modelskill.Definition, caster SkillCaster, target any) (modelskill.ConditionClause, bool) {
	for _, clause := range def.Conditions {
		if !evaluate(clause.Root, caster, target) {
			return clause, false
		}
	}
	return modelskill.ConditionClause{}, true
}

func evaluate(cond modelskill.Condition, caster SkillCaster, target any) bool {
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

func evaluatePlayer(attrs map[string]string, caster SkillCaster) bool {
	for name, raw := range attrs {
		want, err := strconv.ParseBool(strings.ToLower(raw))
		if err != nil {
			return false
		}
		switch strings.ToLower(name) {
		case "flying":
			if caster.Flying() != want {
				return false
			}
		default:
			return false
		}
	}
	return true
}
