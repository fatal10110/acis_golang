package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TransferClanLeaders hands every clan with a pending leader nomination to
// its nominee (clan.Service.HandOverLeadership) and shows each change.
func (l *GameClientLink) TransferClanLeaders() {
	for _, h := range l.clanService().HandOverLeadership() {
		l.showHandover(h)
	}
}

// showHandover shows one leader change, in this order: the former leader
// in the world gets off a flying mount, takes its new clan rank and shows
// it, then takes off what that rank no longer allows; the new leader in
// the world takes its rank and shows it; then every member in the world
// gets its clan window and status refreshed and is told who leads now.
// Each leader's part runs on its own queue and the next part follows it,
// so every member sees the parts in that order.
//
// The siege skills a clan of siege level moves from the former leader to
// the new one wait for the siege engine (#3150).
func (l *GameClientLink) showHandover(h clan.Handover) {
	announce := func() {
		l.broadcastClanStatus(h.Clan)
		name := h.Leader.Name
		l.broadcastToClan(h.Clan, 0, func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanLeaderPrivilegesTransferredToS1, name)
		})
	}
	newLeader := func() {
		l.thenOn(l.onlineMember(h.Leader), func(live *livePlayer) {
			l.clanService().RefreshPledgeClass(live.Character)
			l.broadcastCharacterInfo(live)
		}, announce)
	}
	l.thenOn(l.onlineMember(h.Former), func(live *livePlayer) {
		if live.Character.Flying() {
			live.Character.Dismount()
		}
		l.clanService().RefreshPledgeClass(live.Character)
		l.broadcastCharacterInfo(live)
		l.unequipRestrictedItems(live)
	}, newLeader)
}

// thenOn runs step on live's queue, then next there; with no live player,
// or one whose queue no longer takes work, next runs at once.
func (l *GameClientLink) thenOn(live *livePlayer, step func(*livePlayer), next func()) {
	if live != nil && postLive(live, func() {
		step(live)
		next()
	}) {
		return
	}
	next()
}
