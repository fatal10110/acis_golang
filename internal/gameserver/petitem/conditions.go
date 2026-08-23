package petitem

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// checkUseConditions evaluates an item template's <cond> clauses (all must
// hold) against pet as both caster and target, matching
// Item.checkCondition(pet, pet, true) in RequestPetUseItem.java:40.
func checkUseConditions(pet *summon.Actor, conditions []item.UseCondition) bool {
	for _, uc := range conditions {
		if !petUseConditionHolds(pet, uc.Root) {
			return false
		}
	}
	return true
}

func petUseConditionHolds(pet *summon.Actor, cond item.Condition) bool {
	switch strings.ToLower(cond.Kind) {
	case "and":
		for _, child := range cond.Children {
			if !petUseConditionHolds(pet, child) {
				return false
			}
		}
		return true
	case "or":
		for _, child := range cond.Children {
			if petUseConditionHolds(pet, child) {
				return true
			}
		}
		return false
	case "not":
		return len(cond.Children) == 1 && !petUseConditionHolds(pet, cond.Children[0])
	case "player":
		return petPlayerConditionHolds(pet, cond.Attrs)
	default:
		return false
	}
}

// petPlayerConditionHolds evaluates a <player> leaf's attrs against pet as
// effector. Only "level" is generic across Creature (ConditionPlayerLevel,
// ConditionPlayerLevel.java:19: effector.getStatus().getLevel()). Every
// other <player> attribute (sex, isHero, pkCount, ...) casts the effector to
// Player in the reference (e.g. ConditionPlayerSex.java:20,
// ConditionPlayerIsHero.java:20); a pet is never a Player, so those clauses
// always fail.
func petPlayerConditionHolds(pet *summon.Actor, attrs map[string]string) bool {
	for name, raw := range attrs {
		switch strings.ToLower(name) {
		case "level":
			level, err := strconv.Atoi(raw)
			if err != nil || pet.Level() < level {
				return false
			}
		default:
			return false
		}
	}
	return true
}
