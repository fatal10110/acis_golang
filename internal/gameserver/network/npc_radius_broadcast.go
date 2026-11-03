package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// broadcastInRadius fans one lazily built frame out to the players within
// radius of t, bodies included, searching as many regions around t as
// radius spans.
func broadcastInRadius(state *world.State, t world.Tracked, radius int, build func() wire.Frame) {
	broadcastFrame(build, func(send func(frameReceiver)) {
		state.ForEachKnownInRadius(t, radius, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}
