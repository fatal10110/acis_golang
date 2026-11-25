package network

import (
	"context"
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
//
// A new image is written to disk off the queue, on the connection
// goroutine (deferLookup), and the clan points at it back on the queue,
// where the upload is answered.
func (l *GameClientLink) requestSetClanCrest(live *livePlayer, typ datacache.CrestType, req clientpackets.CrestUpload, limit int32) {
	if req.Length > limit {
		return
	}
	svc := l.clanService()
	cl, result, up := svc.SetCrest(live.Character, typ, req.Data, l.crests, time.Now())
	if result != clan.CrestPending {
		l.answerClanCrest(live, cl, result)
		return
	}
	live.deferLookup(func(context.Context) {
		if !svc.StoreCrest(up, l.crests) {
			return
		}
		onLive(live, func() {
			cl, result := svc.CommitCrest(live.Character, up, l.crests, time.Now())
			l.answerClanCrest(live, cl, result)
		})
	})
}

// answerClanCrest answers live's clan crest upload with result: a set or
// deleted crest refreshes every online member of cl before live is told.
func (l *GameClientLink) answerClanCrest(live *livePlayer, cl *clan.Clan, result clan.CrestResult) {
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
