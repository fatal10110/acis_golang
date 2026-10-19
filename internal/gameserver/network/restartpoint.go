package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// restartTeleportOffset is the random scatter radius applied to a restart
// destination, matching the fixed offset the client-visible restart flow
// uses.
const restartTeleportOffset = 20

// restartLivePlayer handles a dead player's restart-point selection: it
// resolves the destination the request type names, takes the player out of
// any Seven Signs dungeon, revives it (a clan hall restart first restoring
// the experience its hall's restore-exp function gives back), and teleports
// it there. A selection the player may not use is ignored,
// leaving the player dead on the death screen.
//
// A dead attacker of the siege in progress where it lies waits out the
// attacker respawn delay first; the restart then resolves as the player
// stands when the wait ends.
func (l *GameClientLink) restartLivePlayer(live *livePlayer, req clientpackets.RequestRestartPoint) {
	if live == nil {
		return
	}
	// A request while playing dead, a corpse still getting up out of fake
	// death included, only gets up out of it.
	if live.FakeDead() {
		live.GetUpFromFakeDeath()
		return
	}
	if !live.Dead() {
		return
	}
	if l.siegeAttackerAt(live) {
		l.delaySiegeRestart(live, req.RequestType)
		return
	}
	l.portDeadPlayer(live, req.RequestType)
}

// siegeAttackerAt reports whether live's clan attacks the siege in progress
// at live's position.
func (l *GameClientLink) siegeAttackerAt(live *livePlayer) bool {
	clanID := live.ClanID()
	if clanID == 0 {
		return false
	}
	at := live.CurrentLocation()
	return l.sieges.AttackerAt(clanID, at.X, at.Y, at.Z)
}

// delaySiegeRestart runs the restart of requestType once the siege
// attacker respawn delay has passed, on live's queue. Nothing answers the
// request meanwhile: the revive and teleport that end the wait are its
// answer, and the death window stays up until then. A request made while
// one is already waiting is dropped: the waiting one restarts the player,
// and a second one would only teleport the player again once revived. The
// wait ends with nothing done when the player was revived meanwhile or has
// left the world.
func (l *GameClientLink) delaySiegeRestart(live *livePlayer, requestType int32) {
	if live.siegeRestart != nil {
		return
	}
	live.siegeRestart = live.after(l.sieges.AttackerRespawn(), func() {
		live.siegeRestart = nil
		if live.detached() || !live.Dead() {
			return
		}
		l.portDeadPlayer(live, requestType)
	})
}

// portDeadPlayer restarts the dead live as a request of requestType asks.
// It runs on live's queue.
func (l *GameClientLink) portDeadPlayer(live *livePlayer, requestType int32) {
	requestType = effectiveRestartType(live, requestType)
	dest, outcome := l.restartPointDestination(live, requestType)
	switch outcome {
	case restartRefused:
		return
	case restartNoPoint:
		// This is a data-loading gap (no restart-point table loaded at
		// all), not a normal rejection: the nearest-town lookup has nothing
		// to return, and nothing downstream handles that safely.
		// With no destination and nothing sent, the dead player is stranded
		// on the death screen; ActionFailed is the minimum that lets the
		// client dismiss the pending death action so the player isn't
		// stuck, while the warn still surfaces the missing data.
		l.log.Warn().Int32("object_id", live.ObjectID()).Msg("game client: no restart point resolved")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	live.SetIn7sDungeon(false)
	if restore := l.restartRestoreExp(live, requestType); restore > 0 {
		live.ReviveRestoringExp(float64(restore))
	} else {
		live.Revive()
	}
	l.teleportLivePlayer(live, dest, restartTeleportOffset)
}

// restartRestoreExp is the percentage of the experience its last death took
// that a restart of requestType gives back to live: the level of the
// restore-exp function its clan's hall rents, for a clan hall restart only,
// 0 for none. It is restored before the revive.
func (l *GameClientLink) restartRestoreExp(live *livePlayer, requestType int32) int {
	if requestType != restartToClanHall {
		return 0
	}
	hallID := live.ClanHallID()
	if hallID == 0 {
		return 0
	}
	return l.hallFunctions.Level(hallID, hallmodel.FuncRestoreExp)
}

// restartDestination is where a restart, recall or other teleport to town sends
// live, jailed or not, past the residence branches: during a siege in progress
// at live's position, while Dawn holds the Seal of Strife in seal validation, a
// random spawn of the besieged castle, a CHAOTIC one for a player with karma
// and an OTHER one otherwise; else live's nearest town restart point. A castle
// listing no such spawn falls back to the nearest town.
//
// ponytail: the FlagWar clan restart point of a clan hall siege waits for
// the FlagWar hall sieges to store each clan's choice (#3502).
func (l *GameClientLink) restartDestination(live *livePlayer) (location.Location, bool) {
	if s, ok := l.siegeAt(live); ok && l.dawnHoldsStrife() {
		kind := residence.SpawnOther
		if live.Karma() > 0 {
			kind = residence.SpawnChaotic
		}
		if c, ok := l.castleData.Get(s.Castle().ID); ok {
			if at, ok := randomSpawn(c.Spawns, kind); ok {
				return at, true
			}
		}
	}
	if l.restarts == nil {
		return location.Location{}, false
	}
	return l.restarts.NearestLocation(live.CurrentLocation(), live.Race, live.Karma())
}

// teleportLivePlayer relocates live to a scattered point near target, snapped
// to ground height unless live is flying or the point is inside water,
// cancelling any attack, cast or move in progress and resetting the
// selection while keeping the attack stance. It broadcasts the
// discontinuous-position packet to live's own session and every observer,
// then takes live off the grid: it exits every
// zone around the old position, everything around the old position forgets
// live and live forgets it, even what the destination still sees. live
// rejoins the grid and enters the destination's zones once its client
// reports it appeared (completeLivePlayerTeleport). A player lingering after
// its connection dropped has no client to report it, so it lands at once,
// before the enchant, trade and boat cancels. The caller runs on live's queue.
func (l *GameClientLink) teleportLivePlayer(live *livePlayer, target location.Location, randomOffset int) {
	live.teleportMu.Lock()
	locked := true
	defer func() {
		if locked {
			live.teleportMu.Unlock()
		}
	}()
	if !l.relocateLivePlayer(live, target, randomOffset) {
		return
	}
	if live.clientDetached() && !live.detached() {
		landed, followed := l.landTeleportedPlayer(live)
		// The NPCs around the landing point notice the summon that followed
		// and live itself with the teleport lock released: their
		// see-creature hooks may teleport live again.
		live.teleportMu.Unlock()
		locked = false
		if landed {
			l.tellLandingOnlookers(live, followed)
		}
		live.teleportMu.Lock()
		locked = true
	}
	// A completed teleport drops the scroll-of-enchant selection, then ends
	// any open trade for both sides, naming the one who moved, then takes
	// a passenger off its boat.
	l.cancelActiveEnchant(live)
	l.cancelActiveTrade(live)
	l.leaveBoat(live)
}

// relocateLivePlayer is teleportLivePlayer up to the position update, under
// live's teleport lock. It reports whether live was not already teleporting.
func (l *GameClientLink) relocateLivePlayer(live *livePlayer, target location.Location, randomOffset int) bool {
	if !live.SetTeleporting(true) {
		return false
	}
	// Fusion channels targeting live are left alone here: fusion has no
	// teleport hook, only death/class-change/logout/party abort triggers.
	// An out-of-range/LOS teleport is instead caught within ≤1s by the
	// existing fixed-rate FusionChannelValid recheck.
	// live.abortAll(true) already reaches the cast controller (it calls
	// p.cast.Stop()), so a second StopCast() here would hit the same
	// Controller instance twice — castController wires live.Character's
	// cast controller to the identical *actorcast.Controller stored in
	// live.cast — and double the unconditional action-failed ack
	// (controller.go's stopInternal fires onStopAck on every call
	// regardless of whether a cast was in flight).
	//
	// The attack stance survives the teleport: no AutoAttackStop here, the
	// stance ends on its own expiry.
	live.abortAll(true)
	// Every hostile around the old position forgets live, dropping its
	// threat even when the destination is still in that hostile's sight.
	npc.DropThreatAround(l.world, live)
	target = move.RandomNearbyLocation(l.geo, target, randomOffset, func(at location.Location) bool {
		return live.Flying() || l.inWater(at)
	})
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
		// The abort already reset the selection. One made since is
		// forgotten with the old neighborhood, while it still sees live so
		// its observers get the TargetUnselected too; live never knows
		// itself, so a self-selection is not forgotten here.
		if selected := live.Target(); selected != nil && selected.ObjectID() != live.ObjectID() && world.Knows(live, selected) {
			live.forgetTarget(selected)
		}
		l.world.Leave(live)
	}
	// Off the grid the player is in no water, so the breath countdown stops
	// here, after the old neighborhood is forgotten.
	if l.water != nil {
		l.water.Remove(live)
	}
	if live.zoneActor != nil {
		live.zoneActor.sendCurrentCompass()
	}
	l.updateLivePlayerPosition(live, target, heading)
	return true
}

// tellLandingOnlookers has the NPCs around a lingering player's landing
// point run their see-creature hooks (npc.ForceSeeCreature) for the summon
// that followed it, nil for none, then for the player.
func (l *GameClientLink) tellLandingOnlookers(live *livePlayer, followed *summon.Actor) {
	if followed != nil {
		npc.ForceSeeCreature(l.world, followed)
	}
	npc.ForceSeeCreature(l.world, live)
}

// inWater reports whether at lies inside a water zone, where a creature
// swims at its own height instead of standing on the ground below.
func (l *GameClientLink) inWater(at location.Location) bool {
	if l.zones == nil {
		return false
	}
	_, ok := zone.FindAt[*zone.Water](l.zones, at.X, at.Y, at.Z)
	return ok
}

// completeLivePlayerTeleport lands live once its client reports it appeared:
// live rejoins the grid and its zones, and its summon jumps to it.
//
// A summon that followed live tells the NPCs around its landing point about
// itself (npc.ForceSeeCreature) once the teleport lock is released, as its
// see-creature hooks may teleport live again. live's own landing tells
// nobody: the see-creature re-fire runs before an Appearing gives a
// player its region back.
func (l *GameClientLink) completeLivePlayerTeleport(live *livePlayer) {
	if live == nil {
		return
	}
	if _, followed := l.landLivePlayer(live); followed != nil {
		npc.ForceSeeCreature(l.world, followed)
	}
}

// landLivePlayer is completeLivePlayerTeleport under live's teleport lock.
// It reports whether live was teleporting and landed, and returns the summon
// that jumped to it, nil for none.
func (l *GameClientLink) landLivePlayer(live *livePlayer) (bool, *summon.Actor) {
	live.teleportMu.Lock()
	defer live.teleportMu.Unlock()
	return l.landTeleportedPlayer(live)
}

// landTeleportedPlayer is landLivePlayer with live's teleport lock held.
func (l *GameClientLink) landTeleportedPlayer(live *livePlayer) (bool, *summon.Actor) {
	if !live.SetTeleporting(false) {
		return false, nil
	}
	if l.world != nil {
		l.world.Rejoin(live)
	}
	l.rejoinZones(live)
	l.activateSpawnProtection(live)
	closeStoreOnTeleport(live)
	if l.world == nil {
		return true, nil
	}
	active, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return true, nil
	}
	actor, ok := active.(*summon.Actor)
	if !ok {
		return true, nil
	}
	destination := live.CurrentLocation()
	// The summon leaves the zones around its old position before the jump
	// and enters those at the destination once it has landed.
	actor.LeaveZones()
	// Leaving its zones takes a siege summon off its battlefield, so its
	// jump dismisses it wherever it lands, and its owner hears it vanished.
	vanishes := actor.OffBattlefield()
	actor.AbortForTeleport()
	// The summon's jump is a teleport of its own: hostiles around its old
	// position forget it too.
	npc.DropThreatAround(l.world, actor)
	l.broadcastSummonFrame(actor, serverpackets.FrameTeleportToLocation(actor.ObjectID(), destination, false))
	if vanishes {
		// Dismissed before it lands: nobody at its landing point, its owner
		// included, is ever shown it there. A corpse cannot be dismissed,
		// so it only leaves the grid; its owner is told it vanished all
		// the same. The dismissal its battlefield exit posted then finds it
		// gone.
		if actor.Dead() {
			l.world.Leave(actor)
		}
		actor.Vanish()
		return true, nil
	}
	actor.Move().SetPosition(destination)
	_ = l.world.Teleport(actor, destination.X, destination.Y, destination.Z)
	actor.EnterZones()
	actor.OnTeleported()
	l.showSummonCrestToOwner(live, actor)
	return true, actor
}
