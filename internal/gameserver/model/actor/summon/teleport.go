package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TeleportTo moves the summon to target in an ordinary teleport, on its
// queue once the caller's task is done; the network layer runs the jump.
func (a *Actor) TeleportTo(target location.Location) {
	a.emit(event.TeleportRequested{X: target.X, Y: target.Y, Z: target.Z})
}
