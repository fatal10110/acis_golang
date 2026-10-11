package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// InstantTeleportTo moves the summon in one step to where j lands, on its
// queue once the caller's task is done; LandInstantly finishes the jump.
func (a *Actor) InstantTeleportTo(j location.Jump) {
	a.emit(event.InstantTeleportRequested{Jump: j})
}

// LandInstantly puts the summon at target, once its observers have seen the
// jump: it stays in its region until it next moves, so what knows it and
// what it knows are unchanged, its zones are revalidated as for a position
// set outside movement, and it follows its owner again.
func (a *Actor) LandInstantly(target location.Location) {
	previous := a.location()
	a.Move().SetPosition(target)
	if a.world != nil {
		a.world.SetPosition(a, target.X, target.Y, target.Z)
		a.membership.place(previous)
	}
	a.OnTeleported()
}
