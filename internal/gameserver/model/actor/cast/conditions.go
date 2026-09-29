package cast

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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

// WeaponAllowed reports whether a creature holding the item types in held
// (see Actor.HeldItemTypeMask) may use def: def names no weapon or shield
// type, or held shares a bit with one it names. A name that is no weapon
// or armor type restricts nothing.
func WeaponAllowed(def modelskill.Definition, held int32) bool {
	if def.WeaponsAllowed == "" {
		return true
	}
	allowed := item.ParseWornKindMask(def.WeaponsAllowed)
	return allowed == 0 || held&allowed != 0
}
