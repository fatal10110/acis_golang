package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// requestSetPledgeCrest stores the pledge crest live uploads.
func (l *GameClientLink) requestSetPledgeCrest(live *livePlayer, req clientpackets.CrestUpload) {
	l.requestSetClanCrest(live, datacache.PledgeCrest, req, clientpackets.PledgeCrestMaxLength)
}

// requestSetLargePledgeCrest stores the large pledge crest live uploads.
func (l *GameClientLink) requestSetLargePledgeCrest(live *livePlayer, req clientpackets.CrestUpload) {
	l.requestSetClanCrest(live, datacache.LargePledgeCrest, req, clientpackets.LargePledgeCrestMaxLength)
}

// requestSetClanCrest stores the crest image live uploads as its clan's
// pledge or large pledge crest, typ; an empty image deletes the crest.
//
// An upload declaring more than limit bytes, from a clanless player,
// deleting a crest that is not set, or whose image is not exactly the
// crest's size is answered with nothing, as specified: the crest dialog
// leaves no client action pending.
func (l *GameClientLink) requestSetClanCrest(live *livePlayer, typ datacache.CrestType, req clientpackets.CrestUpload, limit int32) {
	if req.Length > limit {
		return
	}
	cl, result := l.clanService().SetCrest(live.Character, typ, req.Data, l.crests, time.Now())
	var msg int
	switch result {
	case clan.CrestDissolving:
		msg = serverpackets.SystemMessageCannotSetCrestWhileDissolving
	case clan.CrestNotAuthorized:
		msg = serverpackets.SystemMessageNotAuthorizedToDoThat
	case clan.CrestLevelTooLow:
		msg = serverpackets.SystemMessageClanLevel3NeededToSetCrest
	case clan.CrestDeleted:
		l.refreshClanAppearance(live, cl)
		msg = serverpackets.SystemMessageClanCrestHasBeenDeleted
	case clan.CrestRegistered:
		l.refreshClanAppearance(live, cl)
		msg = serverpackets.SystemMessageClanEmblemSuccessfullyRegistered
	default:
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

// refreshClanAppearance resends every online member of cl its UserInfo and
// its CharInfo to the players around it, after a change every member shows,
// such as the clan's crest. live, the member that changed it, is refreshed
// here on its own queue; the others on theirs.
func (l *GameClientLink) refreshClanAppearance(live *livePlayer, cl *clan.Clan) {
	for _, member := range l.onlineClanMembers(cl, 0) {
		if member == live {
			l.broadcastCharacterInfo(live)
			continue
		}
		postLive(member, func() {
			if l.liveInWorld(member) {
				l.broadcastCharacterInfo(member)
			}
		})
	}
}
