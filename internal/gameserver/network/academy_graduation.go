package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// graduateFromAcademy graduates live from its clan's academy as it takes
// classID, when classID is a second occupation and live joined the
// academy; it runs on live's queue, before the occupation changes. The
// clan earns the graduation's reputation, shown to every member live
// included; every member hears of the graduation; live is told its
// membership ended and that it withdrew; the rest see its row deleted and
// its withdrawal; live leaves the clan, with no join penalty, and picks up
// the Academy Circlet.
func (l *GameClientLink) graduateFromAcademy(live *livePlayer, classID int) {
	if tier, _ := player.ClassLevel(classID); tier != 2 {
		return
	}
	g, ok := l.clanService().Graduate(live.Character, time.Now())
	if !ok {
		return
	}
	cl, name, points := g.Clan, live.Name, g.Points
	if g.ReputationMoved {
		// live was still a member as the score moved: it sees the change
		// too, though it is off the roster by now.
		if g.Reputation.Crossed != 0 {
			l.showReputationCrossing(live, cl, g.Reputation.Crossed)
		} else {
			live.SendFrame(framePledgeShowInfoUpdate(cl))
		}
		l.sendReputationChange(cl, g.Reputation, live)
	}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageClanMemberGraduatedFromAcademy, name, int32(points))
	}, func(send func(frameReceiver)) {
		send(live)
		for _, member := range l.onlineClanMembers(cl, 0) {
			send(queuedMember{actor: live, member: member})
		}
	})
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAcademyMembershipTerminated))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouHaveWithdrawnFromClan))
	l.broadcastToClanQueued(cl, live,
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListDelete(name) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1HasWithdrawnFromTheClan, name)
		})
	l.checkLeaverItems(live, cl)
	l.clanService().ApplyGraduated(live.Character, g.Member)
	l.sendLeftClan(live, cl)
	live.AddCreatedItem(clan.AcademyCircletID, 1, l.nextObjectID)
}
