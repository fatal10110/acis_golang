package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// systemMessageNumbersFrame builds the system message id with numbers as
// its parameters, in order.
func systemMessageNumbersFrame(id int, numbers []int32) wire.Frame {
	if len(numbers) == 0 {
		return serverpackets.FrameSystemMessage(id)
	}
	params := make([]serverpackets.SystemMessageParam, len(numbers))
	for i, n := range numbers {
		params[i] = serverpackets.NumberParam(n)
	}
	return serverpackets.FrameSystemMessageParams(id, params...)
}

// addScriptClanReputation adds points to the reputation of live's clan, as
// a script asked: the members in the world see
// the change as on any reputation change, live inline. A clan below level 5
// gains nothing and nobody is told anything; a clanless live does nothing.
func (l *GameClientLink) addScriptClanReputation(live *livePlayer, points int) {
	cl, ok := l.clanService().Table().Get(live.ClanID())
	if !ok {
		return
	}
	if change, changed := l.clanService().AddReputation(cl, points); changed {
		l.sendReputationChange(cl, change, live)
	}
}

// broadcastScriptClanInfo shows every member of live's clan in the world
// the clan's header (PledgeShowInfoUpdate), as a script asked, live inline
// and the others on their own queues,
// behind the reputation change frames already posted there.
func (l *GameClientLink) broadcastScriptClanInfo(live *livePlayer) {
	cl, ok := l.clanService().Table().Get(live.ClanID())
	if !ok {
		return
	}
	l.broadcastToClanQueued(cl, live, func() wire.Frame { return framePledgeShowInfoUpdate(cl) })
}
