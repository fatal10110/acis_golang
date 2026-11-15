package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// petPickUp is the ground-item side of a pet's pick-up intention. Its
// refusals reach the pet's owner, as a summon's packets do.
type petPickUp struct {
	link  *GameClientLink
	actor *summon.Actor
}

// PickUpTarget returns where the ground item objectID lies while it is still
// in the world and on the ground, and whether the pet still sees it.
func (p petPickUp) PickUpTarget(objectID int32) (location.Location, bool, bool) {
	ground, ok := p.link.petPickUpGround(objectID)
	if !ok {
		return location.Location{}, false, false
	}
	x, y, z := ground.Position()
	return location.Location{X: x, Y: y, Z: z}, true, world.Knows(p.actor, ground)
}

// Idle sends the pet idle as its owner's Stop command does.
func (p petPickUp) Idle() { p.actor.TryToIdle() }

// FollowOwner turns the pet's following of its owner back on.
func (p petPickUp) FollowOwner() { p.actor.FollowOwner() }

// PickUp has the pet take the ground item objectID.
func (p petPickUp) PickUp(objectID int32) {
	if owner, ok := liveSummonOwner(p.actor); ok {
		p.link.petTakeGround(owner, p.actor, objectID)
	}
}
