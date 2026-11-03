package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TeleportJump moves the summon in an ordinary teleport to where j lands,
// on its queue once the caller's task is done; the network layer runs the
// jump.
func (a *Actor) TeleportJump(j location.Jump) {
	a.emit(event.SummonTeleportRequested{Jump: j})
}
