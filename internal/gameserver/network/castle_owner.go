package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// siegeVictoryMusic is the music a clan's members hear when it takes a
// castle.
const siegeVictoryMusic = "Siege_Victory"

// setCastleOwner gives c to cl, reporting false when cl already owns a
// castle. The clan that held c loses it; its leader, riding a wyvern, is
// dismounted. cl's members in the world then see its clan header refreshed
// and hear the siege victory music. actor is the player whose queue the
// caller runs on.
//
// ponytail: a siege in progress at the castle moves to its mid-victory
// phase once the owner changes; that needs the siege engine (#234).
func (l *GameClientLink) setCastleOwner(actor *livePlayer, c *castle.Castle, cl *clan.Clan) bool {
	former, ok := l.castles.SetOwner(c, cl)
	if !ok {
		return false
	}
	if former != nil {
		if leader, online := l.livePlayerByID(former.LeaderID()); online {
			onMemberQueue(actor, leader, func() {
				if leader.MountType() == player.MountTypeWyvern {
					leader.Character.Dismount()
				}
			})
		}
	}
	l.broadcastToClan(cl, 0,
		func() wire.Frame { return framePledgeShowInfoUpdate(cl) },
		func() wire.Frame {
			return serverpackets.FramePlaySoundAt(serverpackets.Sound{Type: 1, File: siegeVictoryMusic})
		})
	return true
}

// removeCastleOwner takes c from the clan owning it, reporting false when c
// has none. The clan's members in the world see its clan header refreshed;
// then each takes off what it may no longer wear, while an offline
// member's equipped circlet of c and Lord's Crown go back to its
// inventory. actor is the player whose queue the caller runs on.
//
// ponytail: the castle's dropped mercenary tickets and hired mercenaries
// go with its owner (#238), as do its manor settings (#240) and the clan's
// siege registration (#234), and a siege in progress moves to its
// mid-victory phase instead of the circlets being checked (#234).
func (l *GameClientLink) removeCastleOwner(actor *livePlayer, c *castle.Castle) bool {
	cl, ok := l.castles.RemoveOwner(c)
	if !ok {
		return false
	}
	if cl == nil {
		return true
	}
	l.broadcastToClan(cl, 0, func() wire.Frame { return framePledgeShowInfoUpdate(cl) })
	for _, m := range cl.Members() {
		if live, online := l.livePlayerByID(m.ObjectID); online {
			onMemberQueue(actor, live, func() { l.unequipRestrictedItems(live) })
			continue
		}
		l.castles.UnequipCirclets(c, m.ObjectID)
	}
	return true
}
