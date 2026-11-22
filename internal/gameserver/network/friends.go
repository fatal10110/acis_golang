package network

import (
	"context"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
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

// findCharacter looks a character up by name, under ctx; a failed lookup is
// logged and finds none.
func (l *GameClientLink) findCharacter(ctx context.Context, name string) (relation.Named, bool) {
	if l.characters == nil {
		return relation.Named{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, characterLookupTimeout)
	defer cancel()
	ref, ok, err := l.characters.FindByName(ctx, name)
	if err != nil {
		l.log.Error().Err(err).Str("name", name).Msg("find character by name")
		return relation.Named{}, false
	}
	return ref, ok
}

// whenLiveNamed is whenLive for a request that may name a stored character:
// named returns the name when the request needs that character. The lookup
// reads the database, so it runs here, on the connection goroutine and
// under the connection's context, and not on the player's queue, where it
// would hold a pool worker; handle then runs on the queue with what the
// lookup found. The loop reads no next frame until handle has run, so the
// replies keep their order.
//
// handle may return a further read to make off the queue, as whenLiveStaged's
// begin does; nil when the request is answered.
func whenLiveNamed[T any](
	named func(T) (string, bool),
	handle func(*GameClientLink, *livePlayer, T, relation.Named, bool) func(context.Context),
) func(*GameClientLink, context.Context, *connState, T) error {
	return func(l *GameClientLink, ctx context.Context, cs *connState, req T) error {
		live := cs.live
		if live == nil {
			return nil
		}
		var ref relation.Named
		var found bool
		if name, ok := named(req); ok {
			ref, found = l.findCharacter(ctx, name)
		}
		var lookup func(context.Context)
		onLive(live, func() {
			sim.Attribute(live.Queue(), handle)
			lookup = handle(l, live, req, ref, found)
		})
		if lookup != nil {
			lookup(ctx)
		}
		return nil
	}
}

// friendDelName names the character every RequestFriendDel looks up.
func friendDelName(req clientpackets.RequestFriendDel) (string, bool) { return req.Name, true }

// blockName names the character a block or unblock looks up; the other
// block commands name none.
func blockName(req clientpackets.RequestBlock) (string, bool) {
	return req.Name, req.Type == clientpackets.BlockAdd || req.Type == clientpackets.BlockRemove
}

// characterNames returns the stored names of ids, read under ctx; a failed
// lookup is logged and names none.
func (l *GameClientLink) characterNames(ctx context.Context, ids []int32) map[int32]string {
	if l.characters == nil || len(ids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, characterLookupTimeout)
	defer cancel()
	names, err := l.characters.Names(ctx, ids)
	if err != nil {
		l.log.Error().Err(err).Msg("character names")
		return nil
	}
	return names
}

// friendIDs is id's friend list.
//
// The relation state is nil only on a link built without NewGameClientLink;
// such a link has no friends to list or notify.
func (l *GameClientLink) friendIDs(id int32) []int32 {
	if l.relations == nil {
		return nil
	}
	return l.relations.FriendIDs(id)
}

// friendEntries is the FriendList rows of the friends ids, named from names:
// a friend whose character no longer exists keeps its row with an empty name.
func (l *GameClientLink) friendEntries(ids []int32, names map[int32]string) []serverpackets.FriendListEntry {
	if len(ids) == 0 {
		return nil
	}
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
		friend.SendFrame(serverpackets.FrameL2FriendStatus(online, live.CharacterName(), live.ObjectID()))
		if online {
			friend.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageFriendS1HasLoggedIn, live.CharacterName()))
		}
	}
}

// handleRequestFriendInvite sends the named player a friend invitation.
// Every refusal answers with its message and a failed invitation result.
func (l *GameClientLink) handleRequestFriendInvite(live *livePlayer, req clientpackets.RequestFriendInvite) {
	target, online := l.livePlayerByName(req.Name)
	parties := relation.InviteParties{
		RequesterID:  live.ObjectID(),
		RequesterGM:  live.accessLevel().IsGM,
		TargetOnline: online,
	}
	if online {
		parties.TargetID = target.ObjectID()
		parties.TargetGM = target.accessLevel().IsGM
		parties.TargetBlocked = target.BlockingAll()
	}
	refusal := l.relations.CheckInvite(parties)
	// The invitation takes the target's request slot, shared with every
	// other kind of request; only the target being busy refuses it there.
	if refusal == relation.InviteAllowed &&
		l.tradeBook().Invite(tradebook.KindFriend, live.ObjectID(), target.ObjectID(), false).Status == tradebook.RequestStarted {
		target.SendFrame(serverpackets.FrameFriendAddRequest(live.CharacterName()))
		return
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

// beginAnswerFriendInvite answers the friend invitation live holds.
//
// With no answerable invitation the answer goes unanswered on purpose:
// the invitation dialog closed when the client answered, so no
// click waits on a reply. A request of another kind in live's slot is no
// friend invitation: it stays pending for its own answer.
//
// An accept whose requester's login left names the requester as its
// character is stored: the returned lookup reads that name off the queue
// and sends live's notices; see whenLiveStaged.
func (l *GameClientLink) beginAnswerFriendInvite(live *livePlayer, req clientpackets.RequestAnswerFriendInvite) func(context.Context) {
	inv, ok := l.tradeBook().TakeRequest(tradebook.KindFriend, live.ObjectID())
	if !ok {
		return nil
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
		return nil
	}

	l.relations.AddFriend(inv.RequesterID, live.ObjectID())
	if requester != nil {
		requester.SendFrame(serverpackets.FrameFriendAddRequestResult(true))
		requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AddedToFriends, live.CharacterName()))
		requester.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendAdd, live.CharacterName(), true, live.ObjectID()))
	}
	if requester != nil {
		sendFriendJoined(live, requester.CharacterName(), true, inv.RequesterID)
		return nil
	}
	id := inv.RequesterID
	return func(ctx context.Context) {
		name := l.characterNames(ctx, []int32{id})[id]
		onLive(live, func() { sendFriendJoined(live, name, false, id) })
	}
}

// sendFriendJoined tells live that the character id, named name, joined its
// friend list.
func sendFriendJoined(live *livePlayer, name string, online bool, id int32) {
	live.SendFrame(serverpackets.FrameFriendAddRequestResult(true))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1JoinedAsFriend, name))
	live.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendAdd, name, online, id))
}

// beginFriendList lists live's friends as system messages, each marked
// online or offline, between the list's header and footer. A friend whose
// character no longer exists is left out. The names are read off the queue;
// see whenLiveStaged.
func (l *GameClientLink) beginFriendList(live *livePlayer) func(context.Context) {
	ids := l.relations.FriendIDs(live.ObjectID())
	return func(ctx context.Context) {
		names := l.characterNames(ctx, ids)
		onLive(live, func() { l.sendFriendList(live, ids, names) })
	}
}

// sendFriendList sends live the friend list of ids, named from names.
func (l *GameClientLink) sendFriendList(live *livePlayer, ids []int32, names map[int32]string) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFriendListHeader))
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
// USER_NOT_IN_FRIENDS_LIST. ref is the character the name finds, when found.
func (l *GameClientLink) handleRequestFriendDel(live *livePlayer, req clientpackets.RequestFriendDel, ref relation.Named, found bool) func(context.Context) {
	if !found || !l.relations.RemoveFriend(live.ObjectID(), ref.ID) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageUserNotInFriendsList))
		return nil
	}
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, ref.Name))
	if target, online := l.livePlayerByName(req.Name); online {
		live.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendRemove, target.CharacterName(), true, target.ObjectID()))
		target.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendRemove, live.CharacterName(), true, live.ObjectID()))
		return nil
	}
	live.SendFrame(serverpackets.FrameL2Friend(serverpackets.L2FriendRemove, req.Name, false, 0))
	return nil
}

// handleRequestBlock runs one block-list command.
//
// Two branches answer nothing on purpose: an unblock of a name not on the
// block list, and a command type no command sends. Neither registers
// a pending client action; the chat-line commands expect no reply.
//
// ref is the character a block or unblock names, when found. The block list
// names its characters off the queue: it returns that read.
func (l *GameClientLink) handleRequestBlock(live *livePlayer, req clientpackets.RequestBlock, ref relation.Named, found bool) func(context.Context) {
	switch req.Type {
	case clientpackets.BlockAdd, clientpackets.BlockRemove:
		switch relation.CheckBlockTarget(live.ObjectID(), ref, found) {
		case relation.BlockInvalidTarget:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFailedToRegisterToIgnoreList))
			return nil
		case relation.BlockTargetGM:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouMayNotImposeBlockOnGM))
			return nil
		}
		if req.Type == clientpackets.BlockRemove {
			if l.relations.Unblock(live.ObjectID(), ref.ID) {
				live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1RemovedFromYourIgnoreList, ref.Name))
			}
			return nil
		}
		// The block is recorded before either notice goes out: the blocked
		// player's connection runs on its own goroutine, so a request it
		// sends after reading the notice must already find the block.
		l.relations.Block(live.ObjectID(), ref.ID)
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AddedToYourIgnoreList, ref.Name))
		if target, online := l.livePlayerByID(ref.ID); online {
			target.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1HasAddedYouToIgnoreList, live.CharacterName()))
		}

	case clientpackets.BlockList:
		return l.beginBlockList(live)

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
	return nil
}

// beginBlockList lists live's block list, one numbered line per character,
// between the block list's header and the list footer. A blocked character
// that no longer exists is listed as "null". The names are read off the
// queue, by the lookup it returns.
func (l *GameClientLink) beginBlockList(live *livePlayer) func(context.Context) {
	ids := l.relations.BlockedIDs(live.ObjectID())
	return func(ctx context.Context) {
		names := l.characterNames(ctx, ids)
		onLive(live, func() { sendBlockList(live, ids, names) })
	}
}

// sendBlockList sends live its block list of ids, named from names.
func sendBlockList(live *livePlayer, ids []int32, names map[int32]string) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBlockListHeader))
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
// An empty message, or one over friendSayMaxLength, goes unanswered on
// purpose; the client shows what it typed itself and waits on no
// reply.
//
// A delivered message is recorded in the chat log.
func (l *GameClientLink) handleRequestSendL2FriendSay(live *livePlayer, req clientpackets.RequestSendL2FriendSay) {
	if req.Message == "" || commons.UTF16Len(req.Message) > friendSayMaxLength {
		return
	}
	recipient, online := l.livePlayerByName(req.Recipient)
	if !online || !l.relations.AreFriends(live.ObjectID(), recipient.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if l.relations.IsBlocked(recipient.ObjectID(), live.ObjectID()) {
		live.SendFrame(serverpackets.FrameL2FriendSay(serverpackets.SystemMessageS1HasAddedYouToIgnoreList2, live.CharacterName(), req.Recipient, req.Message))
		return
	}
	l.chat.Log.Info().Msg(chat.FriendLogEntry(live.CharacterName(), req.Recipient, req.Message))
	recipient.SendFrame(serverpackets.FrameL2FriendSay(0, req.Recipient, live.CharacterName(), req.Message))
}
