package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// InstantTeleportTo moves c to target in one step, keeping it on the world
// grid: the jump a script makes when it swaps two creatures' places.
func (c *Character) InstantTeleportTo(target location.Location) {
	c.emit(event.InstantTeleportRequested{To: target})
}
