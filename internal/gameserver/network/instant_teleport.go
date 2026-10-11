package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// instantTeleportLivePlayer moves live in one step to where j lands, as the
// reference's instant teleport does: the landing is grounded unless live
// flies or it lies in water; live and its observers get the jump without
// its fade; live stays in its region until its next position update, so
// what knows it and what it knows are unchanged. live then goes idle, and,
// once the teleport lock is released (their see-creature hooks may move
// live again), the NPCs around the region live jumped from that see the
// landing notice it. A player already in a teleport is left to it.
func (l *GameClientLink) instantTeleportLivePlayer(live *livePlayer, j location.Jump) {
	if from, ok := l.jumpLivePlayer(live, j); ok {
		npc.ForceSeeCreatureFrom(l.world, live, from)
	}
}

// jumpLivePlayer is instantTeleportLivePlayer under live's teleport lock,
// up to the NPCs' notice; it reports where live jumped from and whether it
// jumped.
func (l *GameClientLink) jumpLivePlayer(live *livePlayer, j location.Jump) (location.Location, bool) {
	live.teleportMu.Lock()
	defer live.teleportMu.Unlock()
	if live.Teleporting() {
		return location.Location{}, false
	}
	from := live.CurrentLocation()
	target := l.landing(j, func(at location.Location) bool {
		return live.Flying() || l.inWater(at)
	})
	heading := live.CurrentHeading()
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameTeleportToLocation(live.ObjectID(), target, true)
	})
	l.placeLivePlayer(live, target, heading, true)
	live.goIdle()
	return from, true
}

// instantTeleportSummon moves actor in one step to where j lands, on its
// queue: the landing is grounded unless it lies in water, its observers
// get the jump without its fade, it stays in its region until it next
// moves and follows its owner again, and the NPCs around the region it
// jumped from that see the landing notice it. A summon in a teleport
// already, or gone from the world, is left as it is.
func (l *GameClientLink) instantTeleportSummon(actor *summon.Actor, j location.Jump) {
	if actor.Teleporting() || !actor.Spawned() {
		return
	}
	x, y, z := actor.Position()
	from := location.Location{X: x, Y: y, Z: z}
	target := l.landing(j, l.inWater)
	l.broadcastSummonFrame(actor, serverpackets.FrameTeleportToLocation(actor.ObjectID(), target, true))
	actor.LandInstantly(target)
	npc.ForceSeeCreatureFrom(l.world, actor, from)
}

// landing returns where j lands on the server's geodata; keepHeight
// reports a point that keeps its own height.
func (l *GameClientLink) landing(j location.Jump, keepHeight func(location.Location) bool) location.Location {
	if l.geo == nil {
		return j.To
	}
	return move.Land(l.geo, j, keepHeight)
}
