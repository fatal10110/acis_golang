package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// PreparedForReuseMessage reports that skill SkillID at Level is ready to
// be used again: the answer of an item-creating skill that names nothing
// to create.
type PreparedForReuseMessage struct {
	SkillID modelskill.ID
	Level   int
}

// createItemHandler gives a CREATE_ITEM skill's product to its player
// caster. Without ids it cannot create items and does nothing.
type createItemHandler struct{ ids objectIDAllocator }

func (createItemHandler) Types() []string { return []string{"CREATE_ITEM"} }

// Use picks one of the skill's item ids and a count at random and creates
// them in a living player caster's inventory, named in chat as picked up.
// Slots and weight are not checked: the skill's own conditions guard them.
// A skill naming no ids or a zero count creates nothing and reports
// PreparedForReuseMessage; one whose random count is below 1 creates
// nothing either.
func (h createItemHandler) Use(cast Cast) {
	if h.ids == nil || cast.Caster == nil || cast.Caster.Kind() != actor.KindPlayer || alikeDead(cast.Caster) {
		return
	}
	caster, ok := cast.Caster.(itemCreator)
	if !ok {
		return
	}
	def := cast.Skill
	if len(def.CreateItemIDs) == 0 || def.CreateItemCount == 0 {
		cast.record(PreparedForReuseMessage{SkillID: def.ID, Level: def.Level})
		return
	}
	// No extra count can be drawn below 1: such a skill creates nothing.
	if def.CreateItemRandomCount < 1 {
		return
	}
	itemID := def.CreateItemIDs[rnd.Get(len(def.CreateItemIDs))]
	caster.AddCreatedItem(itemID, def.CreateItemCount+rnd.Get(def.CreateItemRandomCount), h.ids.NextID)
}
