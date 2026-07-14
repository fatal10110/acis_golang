package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type itemAdder interface {
	AddItem(itemID int32, count int)
}

// inventoryCapacityChecker optionally lets the caster reject an
// extraction that would overflow their inventory; a caster without one
// always has room.
type inventoryCapacityChecker interface {
	HasCapacityFor(itemIDs []int32) bool
}

type extractableHandler struct{}

func (extractableHandler) Types() []string { return []string{"EXTRACTABLE", "EXTRACTABLE_FISH"} }

// Use rolls one of the skill's capsule product rows (weighted by percent
// chance out of 100000) and grants every item in it to the caster.
func (extractableHandler) Use(cast Cast) {
	target, ok := cast.Caster.(itemAdder)
	if !ok {
		return
	}

	products := modelskill.ParseExtractableItems(cast.Skill.ExtractableItems)
	if len(products) == 0 {
		return
	}

	chance := rnd.Get(100000)
	for _, product := range products {
		chance -= int(product.Chance * 1000)
		if chance >= 0 {
			continue
		}

		if checker, ok := cast.Caster.(inventoryCapacityChecker); ok {
			ids := make([]int32, len(product.Items))
			for i, it := range product.Items {
				ids[i] = it.ItemID
			}
			if !checker.HasCapacityFor(ids) {
				return
			}
		}

		for _, it := range product.Items {
			target.AddItem(it.ItemID, it.Quantity)
		}
		return
	}
}
