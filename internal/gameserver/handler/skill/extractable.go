package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SlotsFullMessage reports a capsule whose rolled product does not fit the
// caster's inventory slots. NothingInsideMessage reports a capsule whose
// roll picked none of its products.
type (
	SlotsFullMessage     struct{}
	NothingInsideMessage struct{}
)

// itemCreator is a player caster that takes the items an opened capsule
// creates, once the slots they need together were found free.
type itemCreator interface {
	ItemSlotsNeeded(itemID int32, count int) int
	ItemSlotsFit(slots int) bool
	AddCreatedItem(itemID int32, count int, nextID func() (int32, error)) bool
}

// extractableHandler opens capsules. Without ids it cannot create items and
// does nothing.
type extractableHandler struct{ ids objectIDAllocator }

func (extractableHandler) Types() []string { return []string{"EXTRACTABLE", "EXTRACTABLE_FISH"} }

// Use rolls one of the skill's capsule product rows (weighted by percent
// chance out of 100000) and grants every item in it to a player caster.
// A row whose items together need more inventory slots than are free
// grants nothing and reports SlotsFullMessage; a roll that picks no row
// reports NothingInsideMessage. The cast has already spent the capsule
// either way.
func (h extractableHandler) Use(cast Cast) {
	if h.ids == nil || cast.Caster == nil || cast.Caster.Kind() != actor.KindPlayer {
		return
	}
	caster, ok := cast.Caster.(itemCreator)
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

		slots := 0
		for _, it := range product.Items {
			slots += caster.ItemSlotsNeeded(it.ItemID, it.Quantity)
		}
		if !caster.ItemSlotsFit(slots) {
			cast.record(SlotsFullMessage{})
			return
		}

		for _, it := range product.Items {
			caster.AddCreatedItem(it.ItemID, it.Quantity, h.ids.NextID)
		}
		return
	}
	cast.record(NothingInsideMessage{})
}
