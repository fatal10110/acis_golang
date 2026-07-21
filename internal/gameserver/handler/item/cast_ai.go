package item

import (
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ConsumeAICastItemRequest carries what's needed to consume the item
// carrying an already-started AI cast.
type ConsumeAICastItemRequest struct {
	Controller *actorcast.Controller
	Inventory  *itemcontainer.Inventory
	Item       *modelitem.Instance
	Destroyer  InventoryDestroyer
}

// ConsumeAICastItem consumes one unit of req.Item, matching how the
// reference commits an item-driven cast's consumption up front, before
// the cast's broadcast/hit phase, rather than only on a successful hit.
// The caller must have already started the cast (actorcast.
// StartItemSkill) and must call this before broadcasting any cast/launch
// packet. On failure it stops req.Controller, since a half-open cast must
// not linger.
func ConsumeAICastItem(req ConsumeAICastItemRequest) error {
	if _, ok := req.Destroyer.DestroyItem(req.Inventory, req.Item.ObjectID, 1); !ok {
		req.Controller.Stop()
		return actorcast.ErrNotEnoughItems
	}
	return nil
}

// CompleteAICastRequest carries what's needed to apply an already-started,
// already-consumed item-carried cast's hit-phase cost and effects.
type CompleteAICastRequest struct {
	Controller *actorcast.Controller
	Definition modelskill.Definition
	Caster     any
	Target     actorcast.Target
	Effects    actorcast.EffectHandlers
}

// CompleteAICastResult is the outcome of one CompleteAICast call. Err is
// nil on a successful hit; the caller maps it to a rejection reply the
// same way it maps actorcast.StartItemSkill's error.
type CompleteAICastResult struct {
	Err error
}

// CompleteAICast applies the cast's hit-phase cost and, on success, the
// skill's effects to req.Target. It stops req.Controller and reports the
// hit error on failure, so a half-open cast never lingers.
func CompleteAICast(req CompleteAICastRequest) CompleteAICastResult {
	if err := req.Controller.Hit(); err != nil {
		req.Controller.Stop()
		return CompleteAICastResult{Err: err}
	}

	actorcast.ApplyEffects(req.Effects, req.Caster, req.Target, req.Definition)
	req.Controller.Finish()
	return CompleteAICastResult{}
}
