package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// dungeonGate takes live through the Seven Signs dungeon gatekeeper f to
// its instant destination 0, then sets live's dungeon membership to in. A
// gatekeeper with no destinations moves no one but still sets the
// membership; a destination list too short for index 0 stops the command
// at the teleport's own release, before the membership changes. The compass
// shows the new membership once live appears at the destination.
func (l *GameClientLink) dungeonGate(live *livePlayer, f *npc.Folk, in bool) {
	trip := l.gatekeeper.Instant(f.NpcID(), 0)
	l.departFromNpc(live, trip)
	if trip.Release {
		return
	}
	live.SetIn7sDungeon(in)
}

// dungeonRefusal turns live away from the Seven Signs dungeon behind f: the
// message naming the only cabal admitted, when the refusal names one, then
// the refusal page.
func dungeonRefusal(live *livePlayer, f *npc.Folk, reply npc.BypassReply) {
	switch reply.DungeonOnly {
	case sevensigns.Dawn:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCanBeUsedByDawn))
	case sevensigns.Dusk:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCanBeUsedByDusk))
	case sevensigns.NoCabal:
	}
	sendFilledHTML(live, f.ObjectID(), reply.HTML, 0)
}
