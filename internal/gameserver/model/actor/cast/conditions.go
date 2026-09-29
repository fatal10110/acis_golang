package cast

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// ConditionError is a cast refused by one of the skill's <cond> clauses.
// Clause carries the feedback that clause configures.
type ConditionError struct {
	Clause modelskill.ConditionClause
}

func (*ConditionError) Error() string { return "cast: skill condition not met" }

// conditionGate is the optional Actor capability CanCast asks before paying
// item costs: whether def's <cond> clauses hold against target. Player and
// summon casters evaluate them; NPC casts never do.
type conditionGate interface {
	SkillConditions(target Target, def modelskill.Definition) (modelskill.ConditionClause, bool)
}

// SkillConditions evaluates def's <cond> clauses with the player as caster.
func (a PlayerActor) SkillConditions(target Target, def modelskill.Definition) (modelskill.ConditionClause, bool) {
	return conditions.EvaluateSkill(def, a.Character, target)
}

// SkillConditions evaluates def's <cond> clauses with the summon as caster.
func (a SummonActor) SkillConditions(target Target, def modelskill.Definition) (modelskill.ConditionClause, bool) {
	return conditions.EvaluateSkill(def, a.Summon, target)
}
