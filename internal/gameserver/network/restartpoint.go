package network

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
	if live == nil || !live.Dead() {
		return
	}

	dest, ok := l.restartDestination(live)
	if !ok {
		l.log.Warn().Int32("object_id", live.ObjectID()).Msg("game client: no restart point resolved")
		return
	}

	live.Revive(l.respawnRestoreHP)
	l.broadcastLiveRevive(live)
	l.teleportLivePlayer(live, dest, restartTeleportOffset)
}

func (l *GameClientLink) restartDestination(live *livePlayer) (location.Location, bool) {
	if l.restarts == nil {
		return location.Location{}, false
	}
	return l.restarts.NearestLocation(live.CurrentLocation(), live.Race, live.Karma)
}

// teleportLivePlayer relocates live to a scattered, ground-height-snapped
// point near target, cancelling any attack/combat in progress, then
// broadcasts the discontinuous-position packet to live's own session and
// every observer.
func (l *GameClientLink) teleportLivePlayer(live *livePlayer, target location.Location, randomOffset int) {
	live.Stop()
	target = l.scatterTeleportTarget(target, randomOffset)
	l.updateLivePlayerPosition(live, target, live.CurrentHeading())
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameTeleportToLocation(live.ObjectID(), target, false)
	})
}

// scatterTeleportTarget randomizes target within offset units, keeping the
// original point when the scattered one isn't directly reachable, then
// snaps to ground height.
func (l *GameClientLink) scatterTeleportTarget(target location.Location, offset int) location.Location {
	if l.geo == nil {
		return target
	}
	if offset > 0 {
		nx := target.X + rand.IntN(2*offset+1) - offset
		ny := target.Y + rand.IntN(2*offset+1) - offset
		if l.geo.CanMove(target.X, target.Y, target.Z, nx, ny, target.Z) {
			target.X, target.Y = nx, ny
		}
	}
	target.Z = int(l.geo.Height(target.X, target.Y, target.Z))
	return target
}
