package network

import (
	"strings"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// RewardClanOf is the festival world's RewardClanOf: the clan of the
// character named name earns points of reputation, shown to its members online as any
// reputation change is, then each is told that name earned them in the
// festival. A character online answers for its own clan; any other name is
// looked up, in any case, in the clans' rosters. The message goes out even
// to a clan too low in level to earn reputation. Each member gets it on its
// own queue, and then runs once every member's queue has sent it, so the
// period change's announcements that follow come after it.
func (l *GameClientLink) RewardClanOf(name string, points int, then func()) {
	cl, ok := l.clanNamed(name)
	if !ok {
		then()
		return
	}
	if change, changed := l.clanService().AddReputation(cl, points); changed {
		l.sendReputationChange(cl, change, nil)
	}
	l.broadcastToClanQueued(cl, nil, func() wire.Frame {
		return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageFestivalHighestRankedParty, name, int32(points))
	})
	awaitLiveQueues(l.onlineClanMembers(cl, 0), then)
}

// awaitLiveQueues posts a mark behind everything already posted to each of
// lives and runs then on the queue of the last to reach its mark; at once
// on the caller when none takes it.
func awaitLiveQueues(lives []*livePlayer, then func()) {
	var pending atomic.Int32
	pending.Store(int32(len(lives)) + 1)
	reached := func() {
		if pending.Add(-1) == 0 {
			then()
		}
	}
	for _, live := range lives {
		if !postLive(live, reached) {
			reached()
		}
	}
	reached()
}

// clanNamed returns the clan of the character named name: an online
// player's own clan, else the clan whose roster names it in any case.
func (l *GameClientLink) clanNamed(name string) (*clan.Clan, bool) {
	clans := l.clanService().Table()
	if live, ok := l.livePlayerByName(name); ok {
		return clans.Get(live.ClanID())
	}
	if name == "" {
		return nil, false
	}
	for _, cl := range clans.Clans() {
		for _, m := range cl.Members() {
			if strings.EqualFold(m.Name, name) {
				return cl, true
			}
		}
	}
	return nil, false
}

// RemoveBloodOfferings is the festival world's RemoveBloodOfferings:
// every player online, on its own queue, loses the blood offerings it
// carries, told only by its inventory update; then runs after the last.
func (l *GameClientLink) RemoveBloodOfferings(then func()) {
	l.onEachLivePlayerThen(func(live *livePlayer) {
		if inv := live.Inventory(); inv != nil {
			inv.DestroyAll(inv.ItemByTemplateID(festival.BloodOfferingID))
		}
	}, then)
}
