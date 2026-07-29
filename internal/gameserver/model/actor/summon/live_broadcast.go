package summon

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// BroadcastFrame sends frame to every currently known observer capable of
// receiving one (i.e. a connected player session), from this summon's own
// known list. It takes ownership of frame and releases it. It is a no-op until
// SpawnBesideOwner has attached a world.
func (a *Actor) BroadcastFrame(frame wire.Frame) {
	defer frame.Release()
	if a.world == nil {
		return
	}
	a.world.ForEachKnown(a, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		copy, ok := serverpackets.CopyFrame(frame)
		if ok {
			receiver.SendFrame(copy)
		}
	})
}
