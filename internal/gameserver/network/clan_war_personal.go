package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// requestSurrenderPersonally has live give up its clan's war on the clan it
// names, for itself: it takes a full death's experience loss and is told
// under the name it typed; the war then ends once every other member of
// its clan wants peace. A clanless player, or a name no clan bears, is not
// answered, as specified: the request leaves no client action pending.
func (l *GameClientLink) requestSurrenderPersonally(live *livePlayer, req clientpackets.RequestPledgeWarName) {
	war, result := l.clanService().SurrenderPersonally(live.Character, req.PledgeName)
	if result != clan.WarDone {
		l.sendWarRefusal(live, war, result)
		return
	}
	live.Character.ApplyDeathPenalty()
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageYouHavePersonallySurrenderedToS1Clan, req.PledgeName))
	peace := func(objectID int32) (wants, online bool) {
		member, ok := l.livePlayerByID(objectID)
		if !ok {
			return false, false
		}
		return member.Character.WantsPeace(), true
	}
	if l.clanService().EndSurrenderedWar(war, peace, time.Now()) {
		l.broadcastWarEnded(war)
	}
}

// creditClanKill moves the reputation of live's death to a clan member at
// war with its clan, and shows each clan whose score moved its new score.
// The killer's clan is told on its members' queues. live, dying, gets its
// own clan's header in place, behind its death's packets, unless the score
// crossed 0: the clan skills that then change go through each member's
// queue, live's included.
func (l *GameClientLink) creditClanKill(live *livePlayer, e event.ClanKill) {
	kill := l.clanService().CreditWarKill(live.ObjectID(), live.Character.ClanID(), e.KillerID, e.KillerClanID)
	if kill.Gained {
		l.sendReputationChange(kill.Killer, kill.Gain, nil)
	}
	if kill.Lost {
		actor := live
		if kill.Loss.Crossed != 0 {
			actor = nil
		}
		l.sendReputationChange(kill.Victim, kill.Loss, actor)
	}
}
