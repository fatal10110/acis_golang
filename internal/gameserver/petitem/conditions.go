package petitem

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// checkUseConditions evaluates an item template's <cond> clauses (all must
// hold) against pet as both caster and target.
func checkUseConditions(pet *summon.Actor, conditions []item.UseCondition) bool {
	for _, uc := range conditions {
		if !petUseConditionHolds(pet, uc.Root) {
			return false
		}
	}
	return true
}

func petUseConditionHolds(pet *summon.Actor, cond item.Condition) bool {
	return item.EvaluateCondition(cond, func(leaf item.Condition) bool {
		if strings.ToLower(leaf.Kind) != "player" {
			return false
		}
		return petPlayerConditionHolds(pet, leaf.Attrs)
	})
}

// petPlayerConditionHolds evaluates a <player> leaf's attrs against pet as
// effector. Only "level" applies to any creature, read from the effector's
// own level. Every other <player> attribute (sex, isHero, pkCount, ...)
// requires the effector to be a player character; a pet never is, so those
// clauses always fail.
func petPlayerConditionHolds(pet *summon.Actor, attrs map[string]string) bool {
	for name, raw := range attrs {
		switch strings.ToLower(name) {
		case "level":
			level, err := commons.DecodeInt32(raw)
			if err != nil || pet.Level() < int(level) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
