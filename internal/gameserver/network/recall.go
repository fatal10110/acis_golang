package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recallLivePlayer takes live where a recall skill sends it: a castle recall
// lands at a random OWNER spawn of the castle live's clan owns, or of the
// castle besieged at live's position when its clan owns or defends it; a clan
// hall recall at one of the hall live's clan owns; any other recall, or one
// finding no such residence, goes to town (restartDestination). A residence
// with no OWNER spawn moves no one.
//
// ponytail: the FlagWar clan restart point of the town branch waits for the
// hall sieges (#244).
func (l *GameClientLink) recallLivePlayer(live *livePlayer, dest modelskill.RecallType) {
	var (
		at location.Location
		ok bool
	)
	hallID := live.ClanHallID()
	castleID := int32(0)
	if dest == modelskill.RecallCastle {
		castleID = l.restartCastleID(live)
	}
	switch {
	case castleID != 0:
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
	// A party recall reaches live from its caster's queue.
	l.teleportLivePlayerFrom(live, at, restartTeleportOffset, false)
}
