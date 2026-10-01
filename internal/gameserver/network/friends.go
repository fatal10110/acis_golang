package network

import (
	"context"
	"fmt"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
)

// characterDirectory finds characters, online or not, by name and id.
type characterDirectory interface {
	FindByName(ctx context.Context, name string) (relation.Named, bool, error)
	Names(ctx context.Context, ids []int32) (map[int32]string, error)
}

// characterLookupTimeout bounds one directory query a friend or block
// command makes.
const characterLookupTimeout = 5 * time.Second

// friendSayMaxLength is the longest friend message delivered, in UTF-16
// code units.
const friendSayMaxLength = 300

// findCharacter looks a character up by name; a failed lookup is logged
// and finds none.
func (l *GameClientLink) findCharacter(name string) (relation.Named, bool) {
	if l.characters == nil {
		return relation.Named{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), characterLookupTimeout)
	defer cancel()
	ref, ok, err := l.characters.FindByName(ctx, name)
	if err != nil {
		l.log.Error().Err(err).Str("name", name).Msg("find character by name")
		return relation.Named{}, false
	}
	return ref, ok
}

// characterNames returns the stored names of ids; a failed lookup is logged
// and names none.
func (l *GameClientLink) characterNames(ids []int32) map[int32]string {
	if l.characters == nil || len(ids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), characterLookupTimeout)
	defer cancel()
	names, err := l.characters.Names(ctx, ids)
	if err != nil {
		l.log.Error().Err(err).Msg("character names")
		return nil
	}
	return names
}

// friendListEntries is id's friend list as FriendList sends it: a friend
// whose character no longer exists keeps its row with an empty name.
//
// The relation state is nil only on a link built without NewGameClientLink;
// such a link has no friends to list or notify.
func (l *GameClientLink) friendListEntries(id int32) []serverpackets.FriendListEntry {
	if l.relations == nil {
		return nil
	}
	ids := l.relations.FriendIDs(id)
	if len(ids) == 0 {
		return nil
	}
	names := l.characterNames(ids)
	entries := make([]serverpackets.FriendListEntry, len(ids))
	for i, friendID := range ids {
		_, online := l.livePlayerByID(friendID)
		entries[i] = serverpackets.FriendListEntry{ObjectID: friendID, Name: names[friendID], Online: online}
	}
	return entries
}

// notifyFriends tells every online friend of live that it entered or left
// the world; an entry also names it in a system message.
func (l *GameClientLink) notifyFriends(live *livePlayer, online bool) {
	if l.relations == nil {
		return
	}
	for _, id := range l.relations.FriendIDs(live.ObjectID()) {
		friend, ok := l.livePlayerByID(id)
		if !ok {
			continue
		}
		friend.SendFrame(serverpackets.FrameL2FriendStatus(online, live.Name, live.ObjectID()))
		if online {
			friend.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageFriendS1HasLoggedIn, live.Name))
		}
	}
}

// leaveFriends takes live leaving the world out of the friend invitations
// and tells its online friends it left.
func (l *GameClientLink) leaveFriends(live *livePlayer) {
	if l.friendInvites != nil {
		l.friendInvites.Leave(live.ObjectID())
	}
	l.notifyFriends(live, false)
}

// handleRequestFriendInvite sends the named player a friend invitation.
// Every refusal answers with its message and a failed invitation result.
func (l *GameClientLink) handleRequestFriendInvite(live *livePlayer, req clientpackets.RequestFriendInvite) {
	target, online := l.livePlayerByName(req.Name)
	parties := relation.InviteParties{
		RequesterID:  live.ObjectID(),
		RequesterGM:  live.access.IsGM,
		TargetOnline: online,
	}
	if online {
		parties.TargetID = target.ObjectID()
		parties.TargetGM = target.access.IsGM
		parties.TargetBlocked = target.BlockingAll()
	}
	refusal := l.relations.CheckInvite(parties)
	if refusal == relation.InviteAllowed {
		// A pending trade request keeps the target as busy as a pending
		// invitation; one slot for every kind of request is #3153.
		busy := l.trades != nil && l.trades.ProcessingRequest(target.ObjectID())
		if l.friendInvites.Offer(live.ObjectID(), live.Name, target.ObjectID(), busy) {
			target.SendFrame(serverpackets.FrameFriendAddRequest(live.Name))
			return
		}
	}
	live.SendFrame(friendInviteRefusalMessage(refusal, req.Name))
	live.SendFrame(serverpackets.FrameFriendAddRequestResult(false))
}

// friendInviteRefusalMessage is the system message a refused friend
// invitation answers with; InviteAllowed here means the target was busy.
func friendInviteRefusalMessage(refusal relation.InviteRefusal, name string) wire.Frame {
	switch refusal {
	case relation.InviteTargetOffline:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound)
	case relation.InviteSelf:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotAddYourselfToFriendsList)
	case relation.InviteTargetBlockingAll:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1BlockedEverything, name)
	case relation.InviteTargetGM:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessagePlayerIsRejectingFriendInvitations)
	case relation.InviteBlocked:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1HasAddedYouToIgnoreList2, name)
	case relation.InviteAlreadyFriends:
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AlreadyInFriendsList, name)
	default:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageWaitingForAnotherReply)
	}
}

// handleRequestAnswerFriendInvite answers the invitation live holds.
//
// With no answerable invitation the answer goes unanswered, as in the
// reference: the invitation dialog closed when the client answered, so no
// click waits on a reply.
func (l *GameClientLink) handleRequestAnswerFriendInvite(live *livePlayer, req clientpackets.RequestAnswerFriendInvite) {
	inv, ok := l.friendInvites.Answer(live.ObjectID())
	if !ok {
		return
	}
	// The login that invited hears the outcome only while it is still the
	// one in the world.
	var requester *livePlayer
	if !inv.RequesterLeft {
		requester, _ = l.livePlayerByID(inv.RequesterID)
	}
	if req.Response != 1 {
		if requester != nil {
			requester.SendFrame(serverpackets.FrameFriendAddRequestResult(false))
		}
		return
	}

	l.relations.AddFriend(inv.RequesterID, live.ObjectID())
	if requester != nil {
		requester.SendFrame(serverpackets.FrameFriendAddRequestResult(true))
		requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AddedToFriends, live.Name))
		requester.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendAdd, live.Name, true, live.ObjectID()))
	}
	live.SendFrame(serverpackets.FrameFriendAddRequestResult(true))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1JoinedAsFriend, inv.RequesterName))
	live.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendAdd, inv.RequesterName, requester != nil, inv.RequesterID))
}

// handleRequestFriendList lists live's friends as system messages, each
// marked online or offline, between the list's header and footer. A friend
// whose character no longer exists is left out.
func (l *GameClientLink) handleRequestFriendList(live *livePlayer) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFriendListHeader))
	ids := l.relations.FriendIDs(live.ObjectID())
	names := l.characterNames(ids)
	for _, id := range ids {
		name, ok := names[id]
		if !ok {
			continue
		}
		msg := serverpackets.SystemMessageS1Offline
		if _, online := l.livePlayerByID(id); online {
			msg = serverpackets.SystemMessageS1Online
		}
		live.SendFrame(serverpackets.FrameSystemMessageString(msg, name))
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFriendListFooter))
}

// handleRequestFriendDel removes the named character from live's friend
// list, and live from its. A name that is no friend of live answers with
// USER_NOT_IN_FRIENDS_LIST.
func (l *GameClientLink) handleRequestFriendDel(live *livePlayer, req clientpackets.RequestFriendDel) {
	ref, found := l.findCharacter(req.Name)
	if !found || !l.relations.RemoveFriend(live.ObjectID(), ref.ID) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageUserNotInFriendsList))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, ref.Name))
	if target, online := l.livePlayerByName(req.Name); online {
		live.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendRemove, target.Name, true, target.ObjectID()))
		target.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendRemove, live.Name, true, live.ObjectID()))
		return
	}
	live.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendRemove, req.Name, false, 0))
}

// handleRequestBlock runs one block-list command.
//
// Two branches answer nothing, as in the reference: an unblock of a name not
// on the block list, and a command type no command sends. Neither registers
// a pending client action; the chat-line commands expect no reply.
func (l *GameClientLink) handleRequestBlock(live *livePlayer, req clientpackets.RequestBlock) {
	switch req.Type {
	case clientpackets.BlockAdd, clientpackets.BlockRemove:
		ref, found := l.findCharacter(req.Name)
		switch relation.CheckBlockTarget(live.ObjectID(), ref, found) {
		case relation.BlockInvalidTarget:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFailedToRegisterToIgnoreList))
			return
		case relation.BlockTargetGM:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouMayNotImposeBlockOnGM))
			return
		}
		if req.Type == clientpackets.BlockRemove {
			if l.relations.Unblock(live.ObjectID(), ref.ID) {
				live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1RemovedFromYourIgnoreList, ref.Name))
			}
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AddedToYourIgnoreList, ref.Name))
		if target, online := l.livePlayerByID(ref.ID); online {
			target.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1HasAddedYouToIgnoreList, live.Name))
		}
		l.relations.Block(live.ObjectID(), ref.ID)

	case clientpackets.BlockList:
		l.sendBlockList(live)

	case clientpackets.BlockAll, clientpackets.BlockAllRelease:
		on := req.Type == clientpackets.BlockAll
		msg := serverpackets.SystemMessageNotBlockingAll
		if on {
			msg = serverpackets.SystemMessageBlockingAll
		}
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
		live.SetBlockingAll(on)
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))

	default:
		l.log.Warn().Int32("type", req.Type).Int32("object_id", live.ObjectID()).Msg("unknown block type")
	}
}

// sendBlockList lists live's block list, one numbered line per character,
// between the block list's header and the list footer. A blocked character
// that no longer exists is listed as "null".
func (l *GameClientLink) sendBlockList(live *livePlayer) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBlockListHeader))
	ids := l.relations.BlockedIDs(live.ObjectID())
	names := l.characterNames(ids)
	for i, id := range ids {
		name, ok := names[id]
		if !ok {
			name = "null"
		}
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, fmt.Sprintf("%d. %s", i+1, name)))
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFriendListFooter))
}

// handleRequestSendL2FriendSay delivers a private message to an online
// friend. A friend that ignores live sends the message back to live as
// undelivered.
//
// An empty message, or one over friendSayMaxLength, goes unanswered, as in
// the reference; the client shows what it typed itself and waits on no
// reply.
//
// The LogChat chat log does not record the message yet; it lands with the
// chat log itself (#154).
func (l *GameClientLink) handleRequestSendL2FriendSay(live *livePlayer, req clientpackets.RequestSendL2FriendSay) {
	if req.Message == "" || len(utf16.Encode([]rune(req.Message))) > friendSayMaxLength {
		return
	}
	recipient, online := l.livePlayerByName(req.Recipient)
	if !online || !l.relations.AreFriends(live.ObjectID(), recipient.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if l.relations.IsBlocked(recipient.ObjectID(), live.ObjectID()) {
		live.SendFrame(serverpackets.FrameL2FriendSay(serverpackets.SystemMessageS1HasAddedYouToIgnoreList2, live.Name, req.Recipient, req.Message))
		return
	}
	recipient.SendFrame(serverpackets.FrameL2FriendSay(0, req.Recipient, live.Name, req.Message))
}
