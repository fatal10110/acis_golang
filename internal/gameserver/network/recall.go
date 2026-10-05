package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recallLivePlayer takes live where a recall skill sends it
// (RestartPointData.getLocationToTeleport): a castle or clan hall recall
// lands at a random OWNER spawn of the residence live's clan owns, and any
// other recall, or one for a clan owning no such residence, at live's
// nearest town restart point. An owned residence with no OWNER spawn moves
// no one, as the reference's null destination does.
//
// The siege-side rules of that lookup (a castle defender's castle spawn,
// the Seal of Strife and FlagWar town points) come with the sieges (#3346).
func (l *GameClientLink) recallLivePlayer(live *livePlayer, dest modelskill.RecallType) {
	var (
		at location.Location
		ok bool
	)
	castleID, hallID := live.ClanCastleID(), live.ClanHallID()
	switch {
	case dest == modelskill.RecallCastle && castleID != 0:
		at, ok = l.castleOwnerSpawn(castleID)
	case dest == modelskill.RecallClanHall && hallID != 0:
		at, ok = l.clanHallOwnerSpawn(hallID)
	default:
		at, ok = l.restartDestination(live)
	}
	if !ok {
		l.log.Warn().Int32("object_id", live.ObjectID()).Uint8("recall_type", uint8(dest)).
			Msg("recall: no destination resolved")
		return
	}
	l.teleportLivePlayer(live, at, restartTeleportOffset)
}
