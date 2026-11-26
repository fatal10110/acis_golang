package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// titleRefusals answers each refused title request (clan.GrantTitle).
var titleRefusals = map[clan.TitleGrant]int{
	clan.TitleInvalid:       serverpackets.SystemMessageNotWorkingPleaseTryAgainLater,
	clan.TitleNotAuthorized: serverpackets.SystemMessageNotAuthorizedToDoThat,
	clan.TitleClanLevel:     serverpackets.SystemMessageClanLvl3NeededToEndowTitle,
	clan.TitleNotMember:     serverpackets.SystemMessageTargetMustBeInClan,
}

// requestGiveNickName answers RequestGiveNickName (clan.GrantTitle). A
// noble titling itself, or a member titling itself, takes the title at
// once. A member titled by another must be in the world: it takes the title
// on its own queue, and the giver is told the member's new title as it was
// asked for, before the cut to 16 characters.
func (l *GameClientLink) requestGiveNickName(live *livePlayer, req clientpackets.RequestGiveNickName) {
	cl, m, grant := l.clanService().GrantTitle(live.Character, req.Name, req.Title)
	switch grant {
	case clan.TitleSelf:
		clan.SetTitle(live.Character, req.Title)
		l.showTitleChanged(live)
		return
	case clan.TitleMember:
	default:
		live.SendFrame(serverpackets.FrameSystemMessage(titleRefusals[grant]))
		return
	}
	var target *livePlayer
	if m.Online {
		target, _ = l.livePlayerByID(m.ObjectID)
	}
	switch {
	case target == nil:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	case target == live:
		clan.SetTitle(live.Character, req.Title)
		l.showTitleChanged(live)
		return
	}
	// The giver's message goes first, so it precedes the TitleUpdate the
	// giver sees when the member is around it.
	live.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageClanMemberS1TitleChangedToS2,
		serverpackets.TextParam(m.Name), serverpackets.TextParam(req.Title)))
	clanID := cl.ID()
	postLive(target, func() {
		if clan.SetMemberTitle(target.Character, clanID, req.Title) {
			l.showTitleChanged(target)
		}
	})
}

// showTitleChanged tells live, on its queue, that its title changed, and
// shows the new title to it and the players around it.
func (l *GameClientLink) showTitleChanged(live *livePlayer) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTitleChanged))
	l.broadcastTitleInfo(live)
}
