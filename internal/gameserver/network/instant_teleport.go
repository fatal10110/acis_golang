package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// instantTeleportLivePlayer moves live in one step to where j lands, as an
// instant teleport does: the landing is grounded unless live
// flies or it lies in water; live and its observers get the jump without
// its fade; live stays in its region until its next position update, so
// what knows it and what it knows are unchanged. live then goes idle, and,
// once the teleport lock is released (their see-creature hooks may move
// live again), the NPCs around the region live is still held in that see
// the landing notice it. A player already in a teleport is left to it.
func (l *GameClientLink) instantTeleportLivePlayer(live *livePlayer, j location.Jump) {
	if l.jumpLivePlayer(live, j) {
		npc.ForceSeeCreature(l.world, live)
	}
}

// jumpLivePlayer is instantTeleportLivePlayer under live's teleport lock,
// up to the NPCs' notice; it reports whether live jumped.
func (l *GameClientLink) jumpLivePlayer(live *livePlayer, j location.Jump) bool {
	live.teleportMu.Lock()
	defer live.teleportMu.Unlock()
	if live.Teleporting() {
		return false
	}
	target := l.landing(j, func(at location.Location) bool {
		return live.Flying() || l.inWater(at)
	})
	heading := live.CurrentHeading()
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameTeleportToLocation(live.ObjectID(), target, true)
	})
	l.placeLivePlayer(live, target, heading, true)
	live.goIdle()
	return true
}

// instantTeleportSummon moves actor in one step to where j lands, on its
// queue: the landing is grounded unless it lies in water, its observers
// get the jump without its fade, it stays in its region until it next
// moves and follows its owner again, and the NPCs around the region it is
// still held in that see the landing notice it. A summon in a teleport
// already, or gone from the world, is left as it is.
func (l *GameClientLink) instantTeleportSummon(actor *summon.Actor, j location.Jump) {
	if actor.Teleporting() || !actor.Spawned() {
		return
	}
	target := l.landing(j, l.inWater)
	l.broadcastSummonFrame(actor, serverpackets.FrameTeleportToLocation(actor.ObjectID(), target, true))
	actor.LandInstantly(target)
	npc.ForceSeeCreature(l.world, actor)
}

// landing returns where j lands on the server's geodata; keepHeight
// reports a point that keeps its own height.
func (l *GameClientLink) landing(j location.Jump, keepHeight func(location.Location) bool) location.Location {
	if l.geo == nil {
		return j.To
	}
	return move.Land(l.geo, j, keepHeight)
}
