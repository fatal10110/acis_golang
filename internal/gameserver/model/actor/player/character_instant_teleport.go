package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// InstantTeleportTo moves c in one step to where j lands, keeping it in
// its region: the jump a script makes when it swaps two creatures' places.
// Asked from a task of c's own queue (OnOwnQueue), the jump runs inside the
// call; from any other, on c's queue once the calling task is done.
func (c *Character) InstantTeleportTo(j location.Jump) {
	c.emit(event.InstantTeleportRequested{Jump: j, OwnQueue: c.OnOwnQueue()})
}
