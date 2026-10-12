package npc

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// GroundTaker takes out of the world the ground items NPCs pick up.
type GroundTaker interface {
	// TakeGround takes ground, which an NPC's pick-up has claimed, out of
	// the world: a cursed weapon ends its life, any other item leaves the
	// world.
	TakeGround(ground *grounditem.Item)
}

// groundNPC is a live NPC as its ground-item looks and pick-ups see it.
type groundNPC interface {
	attackable.Combatant
	world.Tracked
}

// pickUpTarget returns the ground item objectID while n may still pick it
// up: it is in w, on the ground, and n knows it.
func pickUpTarget(w *world.State, n groundNPC, objectID int32) (*grounditem.Item, bool) {
	if w == nil {
		return nil, false
	}
	obj, ok := w.Object(objectID)
	if !ok {
		return nil, false
	}
	ground, ok := obj.(*grounditem.Item)
	if !ok || !world.Knows(n, ground) {
		return nil, false
	}
	return ground, true
}

// groundAt is where ground lies.
func groundAt(ground *grounditem.Item) location.Location {
	x, y, z := ground.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// pickUp has n pick up the ground item objectID: it claims the item, runs
// the picked-item hooks, then has taker take the item out of the world; a
// nil taker puts it back on the ground. It reports false, having done
// nothing, when the item is no longer there for n or another pickup holds
// it.
func pickUp(w *world.State, n groundNPC, hooks ScriptHooks, taker GroundTaker, objectID int32) bool {
	ground, ok := pickUpTarget(w, n, objectID)
	if !ok || !ground.Claim() {
		return false
	}
	if hooks != nil {
		hooks.PickedItem(n, ground)
	}
	if taker == nil {
		ground.Release()
		return true
	}
	taker.TakeGround(ground)
	return true
}

// lookItem runs n's see-item hooks with quantity and the ground items n
// knows within radius whose item id is one of ids, any item when ids is
// empty. Nothing runs when no item matches.
func lookItem(w *world.State, n groundNPC, hooks ScriptHooks, radius int, quantity int32, ids []int32) {
	if w == nil || hooks == nil {
		return
	}
	var items []*grounditem.Item
	w.ForEachKnownInRadius(n, radius, func(o world.Tracked) {
		if ground, ok := o.(*grounditem.Item); ok && (len(ids) == 0 || slices.Contains(ids, ground.ItemID())) {
			items = append(items, ground)
		}
	})
	if len(items) > 0 {
		hooks.SeeItem(n, quantity, items)
	}
}

// LookItem runs h's see-item hooks with quantity and the ground items h
// knows within radius whose item id is one of ids, any item when ids is
// empty, on the calling goroutine. Nothing runs when no item matches.
func (h *Hostile) LookItem(radius int, quantity int32, ids ...int32) {
	lookItem(h.world, h, h.scripts, radius, quantity, ids)
}

// PickUpTarget returns where the ground item itemObjectID lies while h may
// still pick it up: it is in the world, on the ground, and h knows it.
func (h *Hostile) PickUpTarget(itemObjectID int32) (location.Location, bool) {
	ground, ok := pickUpTarget(h.world, h, itemObjectID)
	if !ok {
		return location.Location{}, false
	}
	return groundAt(ground), true
}

// PickUp has h pick up the ground item itemObjectID: it claims the item,
// runs h's picked-item hooks, then takes the item out of the world, a
// cursed weapon by ending its life. An h attached with no Ground leaves the
// item on the ground. It reports false, having done nothing, when the item
// is no longer there for h or another pickup holds it.
func (h *Hostile) PickUp(itemObjectID int32) bool {
	return pickUp(h.world, h, h.scripts, h.ground, itemObjectID)
}

// LookItem runs f's see-item hooks as Hostile.LookItem does.
func (f *Folk) LookItem(radius int, quantity int32, ids ...int32) {
	lookItem(f.world, f, f.scripts, radius, quantity, ids)
}

// AddPickUpDesire asks f to pick up the ground item itemObjectID with
// weight; an equal desire (same item) already queued gains the weight
// instead. Object id 0 is ignored. Once the item is gone the desire stays
// queued, its step doing nothing. Safe from any goroutine.
func (f *Folk) AddPickUpDesire(itemObjectID int32, weight float64) {
	if itemObjectID == 0 {
		return
	}
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionPickUp, ItemObjectID: itemObjectID, Weight: weight, QueuedAt: f.now()})
}

// thinkPickUp is the pick-up step, under brain: an f that cannot act, or
// whose item it can no longer pick up, does nothing; one not yet within
// ai.PickUpRange walks to the item unless it cannot move. Within range f
// picks the item up, with brain released, then drops the desire of its
// current intention as it stands after the picked-item hooks.
func (f *Folk) thinkPickUp(d *ai.Desire) {
	if f.denyAIAction() {
		return
	}
	ground, ok := pickUpTarget(f.world, f, d.ItemObjectID)
	if !ok {
		return
	}
	if f.location().Distance3D(groundAt(ground)) >= ai.PickUpRange {
		if f.motion != nil && !f.MovementDisabled() {
			_, _ = f.motion.ctl.MoveToLocation(groundAt(ground))
		}
		return
	}
	if f.pickUpUnlocked(d.ItemObjectID) {
		f.clearCurrentDesire()
	}
}

// pickUpUnlocked has f pick up the ground item itemObjectID with brain
// released. The caller holds brain and holds it again on return, a panic
// included.
func (f *Folk) pickUpUnlocked(itemObjectID int32) bool {
	f.cast.brain.Unlock()
	defer f.cast.brain.Lock()
	return pickUp(f.world, f, f.scripts, f.ground, itemObjectID)
}
