package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// instantTeleportLivePlayer moves live to target in one step, as the
// reference's instant teleport does: target is grounded unless live flies
// or target lies in water; live and its observers get the jump without its
// fade; live stays on the grid, so what still sees it keeps knowing it.
// live then goes idle, and, once the teleport lock is released (their
// see-creature hooks may move live again), the NPCs around target notice
// it. A player already in a teleport is left to it.
func (l *GameClientLink) instantTeleportLivePlayer(live *livePlayer, target location.Location) {
	if l.jumpLivePlayer(live, target) {
		npc.ForceSeeCreature(l.world, live)
	}
}

// jumpLivePlayer is instantTeleportLivePlayer under live's teleport lock,
// up to the NPCs' notice; it reports whether live jumped.
func (l *GameClientLink) jumpLivePlayer(live *livePlayer, target location.Location) bool {
	live.teleportMu.Lock()
	defer live.teleportMu.Unlock()
	if live.Teleporting() {
		return false
	}
	target = move.RandomNearbyLocation(l.geo, target, 0, func(at location.Location) bool {
		return live.Flying() || l.inWater(at)
	})
	heading := live.CurrentHeading()
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameTeleportToLocation(live.ObjectID(), target, true)
	})
	l.updateLivePlayerPosition(live, target, heading)
	live.goIdle()
	return true
}
