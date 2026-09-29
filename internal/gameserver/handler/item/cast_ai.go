package item

import (
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ConsumeAICastItemRequest carries what's needed to consume the item
// carrying an AI cast.
type ConsumeAICastItemRequest struct {
	Caster     SkillCaster
	Definition modelskill.Definition
	Inventory  *itemcontainer.Inventory
	Item       *modelitem.Instance
	Template   *modelitem.Template
	Destroyer  InventoryDestroyer
}

// ConsumeAICastItemResult is the outcome of one ConsumeAICastItem call.
// SharedReuseGroup and ReuseMillis are only meaningful when Err is nil:
// SharedReuseGroup is -1 when req.Template defines no shared-reuse group
// (the caller sends no ExUseSharedGroupItem packet in that case).
type ConsumeAICastItemResult struct {
	Err              error
	SharedReuseGroup int32
	ReuseMillis      int
}

// ConsumeAICastItem consumes one unit of req.Item for a cast it carries and
// puts the item-carried skill on its item reuse. It runs once the cast is
// claimed and before the cast's own start-of-cast costs
// (actorcast.StartHooks.ConsumeCarrier): the item is gone, and its reuse
// installed, before the cast's reuse, mastery proc and initial MP are
// charged, which may then replace that reuse with the skill's own.
//
// The item reuse is the longer of the skill's own reuse delay and the
// item's. On success it is also reported for the client's shared-reuse
// indicator, as both the remaining and the total time. A failed destroy
// reports ErrNotEnoughItems and changes nothing.
func ConsumeAICastItem(req ConsumeAICastItemRequest) ConsumeAICastItemResult {
	if _, ok := req.Destroyer.DestroyItem(req.Inventory, req.Item.ObjectID, 1); !ok {
		return ConsumeAICastItemResult{Err: actorcast.ErrNotEnoughItems}
	}

	var itemReuse int32
	sharedReuseGroup := int32(-1)
	if req.Template != nil && req.Template.EtcItem != nil {
		itemReuse = req.Template.EtcItem.ReuseDelay
		sharedReuseGroup = req.Template.EtcItem.SharedReuseGroup
	}
	reuse := installItemReuse(req.Caster, req.Definition, actorcast.ReuseKey(req.Definition), itemReuse)
	return ConsumeAICastItemResult{SharedReuseGroup: sharedReuseGroup, ReuseMillis: reuse}
}
