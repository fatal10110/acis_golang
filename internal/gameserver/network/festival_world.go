package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// A GameClientLink reaches the players and clans as the festival's periods
// change.
var _ festival.World = (*GameClientLink)(nil)

// RewardClanOf implements festival.World: the clan of the character named
// name earns points of reputation, shown to its members online as any
// reputation change is, then each is told that name earned them in the
// festival. A character online answers for its own clan; any other name is
// looked up, in any case, in the clans' rosters. The message goes out even
// to a clan too low in level to earn reputation. Each member gets it on its
// own queue, and the call returns once every member's queue has sent it, so
// the period change's announcements that follow come after it. It must not
// run on a player's queue.
func (l *GameClientLink) RewardClanOf(name string, points int) {
	cl, ok := l.clanNamed(name)
	if !ok {
		return
	}
	if change, changed := l.clanService().AddReputation(cl, points); changed {
		l.sendReputationChange(cl, change, nil)
	}
	l.broadcastToClanQueued(cl, nil, func() wire.Frame {
		return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageFestivalHighestRankedParty, name, int32(points))
	})
	for _, member := range l.onlineClanMembers(cl, 0) {
		awaitLiveQueue(member)
	}
}

// awaitLiveQueue returns once live's queue has run everything posted to it
// so far; at once when the queue has closed. It must not run on a player's
// queue.
func awaitLiveQueue(live *livePlayer) {
	done := make(chan struct{})
	if postLive(live, func() { close(done) }) {
		<-done
	}
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

// RemoveBloodOfferings implements festival.World: every player online,
// on its own queue, loses the blood offerings it carries, told only by its
// inventory update.
func (l *GameClientLink) RemoveBloodOfferings() {
	l.onEachLivePlayer(func(live *livePlayer) {
		if inv := live.Inventory(); inv != nil {
			inv.DestroyAll(inv.ItemByTemplateID(festival.BloodOfferingID))
		}
	})
}
