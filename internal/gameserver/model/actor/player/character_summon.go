package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SummonCreature is the SUMMON_CREATURE skill handler's entry point
// (handler/skill/summon.go's creatureSummonRuntime): only a pet-collar-item
// cast reaches here, so a non-*item.Instance item is a silent no-op, as is
// a missing item or one with no summon data.
func (c *Character) SummonCreature(_ modelskill.Definition, itemArg any) {
	inst, ok := itemArg.(*item.Instance)
	if !ok {
		return
	}
	c.emit(event.PetSummonRequested{ControlItem: inst})
}

// SummonServitor is the non-cubic SUMMON skill handler's entry point.
func (c *Character) SummonServitor(def modelskill.Definition) {
	c.emit(event.ServitorSummonRequested{Skill: def})
}

// ControlItemInUse reports whether objectID is the control item of c's
// summon in the world, alive or dead, or of the mount c rides. Such an item
// is bound to a pet that is out: it may not leave c's inventory by trade,
// drop, destroy, sale, deposit, parcel or a hand-over to the pet itself.
func (c *Character) ControlItemInUse(objectID int32) bool {
	if c == nil || objectID == 0 {
		return false
	}
	if objectID == c.MountObjectID() {
		return true
	}
	if c.world == nil {
		return false
	}
	obj, ok := c.world.Summon(c.ObjectID())
	if !ok {
		return false
	}
	controlled, ok := obj.(interface{ ControlItemID() int32 })
	return ok && controlled.ControlItemID() == objectID
}
