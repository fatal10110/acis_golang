package ai

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// PickUpRange is how close an NPC comes to a ground item before it picks it
// up: strictly within this distance, in 3D, of where the item lies.
const PickUpRange = 36

// AddPickUpDesire queues a request to pick up the ground item itemObjectID
// with weight; an equal desire (same item) already queued gains the weight
// instead. Object id 0 is ignored. The desire carries the item by id only:
// once the item is gone the desire stays queued, its step doing nothing. It
// takes no AI lock, so a hook may call it from any goroutine.
func (a *Attackable) AddPickUpDesire(itemObjectID int32, weight float64) {
	if itemObjectID == 0 {
		return
	}
	a.desires.AddOrUpdate(&Desire{
		Kind:         IntentionPickUp,
		ItemObjectID: itemObjectID,
		Weight:       weight,
		QueuedAt:     a.now(),
	})
}

// thinkPickUp is the pick-up step. An actor that cannot act, or whose item
// it can no longer pick up, does nothing. One not yet within PickUpRange
// walks to the item, unless it cannot move. Within range it picks the item
// up (the picked-item turn runs with mu released), then drops the desire of
// its current intention as it stands after that turn.
func (a *Attackable) thinkPickUp() {
	if a.actor.DenyAIAction() {
		return
	}
	id := a.current.itemObjectID
	at, ok := a.actor.PickUpTarget(id)
	if !ok {
		return
	}
	x, y, z := a.actor.Position()
	if (location.Location{X: x, Y: y, Z: z}).Distance3D(at) >= PickUpRange {
		if !a.actor.MovementDisabled() {
			_, _ = a.move.MoveToLocation(at)
		}
		return
	}
	a.mu.Unlock()
	picked := a.actor.PickUp(id)
	a.mu.Lock()
	if picked {
		a.clearCurrentDesire()
	}
}
