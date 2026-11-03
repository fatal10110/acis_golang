package petitem

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// checkUseConditions tests an item template's use conditions with pet
// using the item on itself. aborted reports a test that reached a <not>
// around no condition: the check fails hard there, before the pet's owner
// is told anything.
func checkUseConditions(pet *summon.Actor, uses []item.UseCondition) (held, aborted bool) {
	actor := pet.ConditionActor()
	_, held, aborted = conditions.EvaluateItem(uses, actor, actor)
	return held, aborted
}
