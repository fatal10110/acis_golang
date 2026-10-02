package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// announceClanGate tells every other online member of live's clan that a
// clan gate portal opened on live; a clanless player tells nobody.
func (l *GameClientLink) announceClanGate(live *livePlayer) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	l.broadcastToClan(cl, live.ObjectID(), func() wire.Frame {
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageCourtMagicianCreatedPortal)
	})
}
