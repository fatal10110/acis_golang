package network

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recallLivePlayer takes live where a recall skill sends it: its nearest
// town restart point. A castle or clan hall recall lands there too while
// live's clan owns no such residence.
//
// A castle or clan hall recall for a clan that owns one belongs at the
// residence's owner spawn, which no residence data here serves yet (#3323);
// until it does, that recall moves no one rather than sending the player to
// a town it did not ask for.
func (l *GameClientLink) recallLivePlayer(live *livePlayer, dest modelskill.RecallType) {
	switch {
	case dest == modelskill.RecallCastle && live.ClanCastleID() != 0,
		dest == modelskill.RecallClanHall && live.ClanHallID() != 0:
		l.log.Warn().Int32("object_id", live.ObjectID()).Uint8("recall_type", uint8(dest)).
			Msg("recall: residence owner spawn not resolved")
		return
	}
	at, ok := l.restartDestination(live)
	if !ok {
		l.log.Warn().Int32("object_id", live.ObjectID()).Msg("recall: no town restart point resolved")
		return
	}
	l.teleportLivePlayer(live, at, restartTeleportOffset)
}
