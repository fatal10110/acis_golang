package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// restartTeleportOffset is the random scatter radius applied to a restart
// destination, matching the fixed offset the client-visible restart flow
// uses.
const restartTeleportOffset = 20

// restartLivePlayer handles a dead player's restart-point selection: it
// resolves a destination, revives the player, and teleports them there.
//
// Clan hall, castle and siege-flag restarts (request types 1-3), the
// GM/festival fixed-position restart (type 4) and the jail restart (type
// 27) all depend on clan/siege ownership, a festival system or a
// punishment system that aren't modeled yet. req.RequestType is accepted
// for wire-format completeness but every request type currently resolves
// to the same destination an unrecognized type would: the player's nearest
// town restart point.
func (l *GameClientLink) restartLivePlayer(live *livePlayer, req clientpackets.RequestRestartPoint) {
	if live == nil {
		return
	}
	if live.FakeDead() {
		live.EffectList().StopByType(effect.TypeFakeDeath)
		return
	}
	if !live.Dead() {
		return
	}

	dest, ok := l.restartDestination(live)
	if !ok {
		// This is a data-loading gap (no restart-point table loaded at
		// all), not a normal rejection the reference path handles safely:
		// its nearest-town lookup can return nil and then dereference it.
		// With no destination and nothing sent, the dead player is stranded
		// on the death screen; ActionFailed is the minimum that lets the
		// client dismiss the pending death action so the player isn't
		// stuck, while the warn still surfaces the missing data.
		l.log.Warn().Int32("object_id", live.ObjectID()).Msg("game client: no restart point resolved")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	live.Revive(l.playerConfig.RespawnRestoreHP)
	l.broadcastLiveRevive(live)
	l.teleportLivePlayer(live, dest, restartTeleportOffset)
}

func (l *GameClientLink) restartDestination(live *livePlayer) (location.Location, bool) {
	if l.restarts == nil {
		return location.Location{}, false
	}
	return l.restarts.NearestLocation(live.CurrentLocation(), live.Race, live.Karma())
}

// teleportLivePlayer relocates live to a scattered, ground-height-snapped
// point near target, cancelling any attack/combat in progress. It broadcasts
// the discontinuous-position packet to live's own session and every
// observer, then takes live off the grid: it exits every zone around the old
// position, everything around the old position forgets live and live forgets
// it, even what the destination still sees. live rejoins the grid and enters
// the destination's zones once its client reports it appeared
// (completeLivePlayerTeleport).
func (l *GameClientLink) teleportLivePlayer(live *livePlayer, target location.Location, randomOffset int) {
	live.teleportMu.Lock()
	defer live.teleportMu.Unlock()
	if !live.SetTeleporting(true) {
		return
	}
	// Fusion channels targeting live are left alone here: the reference has
	// no teleport hook for fusion, only death/class-change/logout/party
	// abort triggers (Player.java:2663,5847,6302; Party.java:202,428). An
	// out-of-range/LOS teleport is instead caught within ≤1s by the existing
	// fixed-rate FusionChannelValid recheck (FusionSkill.java:45-49), same as
	// Java.
	// live.Stop() already reaches the cast controller (live_player.go's
	// Stop() calls p.cast.Stop()), so a second StopCast() here would hit
	// the same Controller instance twice — castController wires
	// live.Character's cast controller to the identical *actorcast.Controller
	// stored in live.cast (live_player.go:270) — and double the
	// unconditional clientActionFailed() ack (PlayerCast.java:382-387,
	// controller.go's stopInternal fires onStopAck on every call
	// regardless of whether a cast was in flight).
	live.Stop()
	// Every hostile around the old position forgets live, dropping its
	// threat even when the destination is still in that hostile's sight.
	npc.DropThreatAround(l.world, live)
	target = move.RandomNearbyLocation(l.geo, target, randomOffset)
	// Off the grid the heading falls back to the saved one; keep the live
	// facing across the jump.
	heading := live.CurrentHeading()
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameTeleportToLocation(live.ObjectID(), target, false)
	})
	// Zone exits come first: a water exit's appearance update still reaches
	// the old neighborhood.
	l.leaveZones(live)
	if l.world != nil {
		// Forgetting a selected object clears the selection. Do it while
		// the old neighborhood still sees live, so its observers get the
		// TargetUnselected too. A self-selection is kept.
		if selected := live.Target(); selected != nil && selected.ObjectID() != live.ObjectID() && world.Knows(live, selected) {
			live.forgetTarget(selected)
		}
		l.world.Leave(live)
	}
	l.updateLivePlayerPosition(live, target, heading)
}

func (l *GameClientLink) completeLivePlayerTeleport(live *livePlayer) {
	if live == nil {
		return
	}
	live.teleportMu.Lock()
	defer live.teleportMu.Unlock()
	if !live.SetTeleporting(false) {
		return
	}
	if l.world != nil {
		l.world.Rejoin(live)
	}
	l.rejoinZones(live)
	l.activateSpawnProtection(live)
	if l.world == nil {
		return
	}
	active, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return
	}
	actor, ok := active.(*summon.Actor)
	if !ok {
		return
	}
	destination := live.CurrentLocation()
	actor.AbortForTeleport()
	// The summon's jump is a teleport of its own: hostiles around its old
	// position forget it too.
	npc.DropThreatAround(l.world, actor)
	l.broadcastSummonFrame(actor, serverpackets.FrameTeleportToLocation(actor.ObjectID(), destination, false))
	actor.Move().SetPosition(destination)
	_ = l.world.Teleport(actor, destination.X, destination.Y, destination.Z)
	actor.OnTeleported()
}
