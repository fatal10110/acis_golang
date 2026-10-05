package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// summonBoatEntrance is the boat-entrance probe of a summon's friendly
// follow of anyone but its owner (Playable.tryToPassBoatEntrance and
// moveToBoatEntrance run for a summon). Its refusals reach the owner, as a
// summon's packets do.
type summonBoatEntrance struct {
	link  *GameClientLink
	actor *summon.Actor
}

// PassBoatEntrance returns the shore point where the summon's walk toward
// dest crosses the entrance of the dock a boat it knows serves, and true
// when that point lies farther than boatEntranceReach from the summon.
// Otherwise the owner is answered ActionFailed.
func (e summonBoatEntrance) PassBoatEntrance(dest location.Location) (location.Location, bool) {
	if b := e.link.knownBoat(e.actor); b != nil {
		x, y, z := e.actor.Position()
		if point, ok := b.Dock().BoardingPoint(boat.Point{X: x, Y: y}, boat.Point{X: dest.X, Y: dest.Y}, false); ok {
			entrance := location.Location{X: point.X, Y: point.Y, Z: boatShoreZ}
			if (location.Location{X: x, Y: y, Z: z}).Distance2D(entrance) > boatEntranceReach {
				return entrance, true
			}
		}
	}
	e.Refuse()
	return location.Location{}, false
}

// Refuse answers the summon's owner ActionFailed.
func (e summonBoatEntrance) Refuse() {
	if owner, ok := liveSummonOwner(e.actor); ok {
		owner.SendFrame(serverpackets.FrameActionFailed())
	}
}
