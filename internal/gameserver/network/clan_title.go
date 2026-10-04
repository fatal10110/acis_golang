package network

import (
	"context"

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
		l.giveTitle(live, req.Title)
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
		l.giveTitle(live, req.Title)
		return
	}
	// The giver's message goes first, so it precedes the TitleUpdate the
	// giver sees when the member is around it, as in the reference.
	live.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageClanMemberS1TitleChangedToS2,
		serverpackets.TextParam(m.Name), serverpackets.TextParam(req.Title)))
	clanID := cl.ID()
	postLive(target, func() {
		// A member that left the clan before its queue got here keeps its
		// title.
		if target.ClanID() == clanID {
			l.giveTitle(target, req.Title)
		}
	})
}

// giveTitle sets live's title to title, cut to 16 characters, on live's
// queue: live is told its title changed, it and the players around it see
// the new title, and the title is stored at once, on live's persistence
// lane, as the periodic character store does not write it.
func (l *GameClientLink) giveTitle(live *livePlayer, title string) {
	title = trimTitle(title)
	live.SetTitle(title)
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTitleChanged))
	l.broadcastTitleInfo(live)
	id := live.ObjectID()
	l.storeCharacterEdit(id, "store title", func(ctx context.Context, s characterEditStore) error {
		return s.SetTitle(ctx, id, title)
	})
}
