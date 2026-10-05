package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// friendsCleared is the message clearing the friends list on the board
// answers with.
const friendsCleared = "You have cleared your friends list."

// boardContacts looks up the characters a friends board page lists. Names
// are read once per page.
type boardContacts struct {
	link  *GameClientLink
	names map[int32]string
}

func (l *GameClientLink) boardContacts(ids ...[]int32) boardContacts {
	var all []int32
	for _, list := range ids {
		all = append(all, list...)
	}
	return boardContacts{link: l, names: l.characterNames(all)}
}

func (c boardContacts) Name(id int32) (string, bool) {
	name, ok := c.names[id]
	return name, ok
}

func (c boardContacts) Online(id int32) bool {
	_, ok := c.link.livePlayerByID(id)
	return ok
}

// boardFriends runs a friends board command: _friendlist and _blocklist
// show the lists; _friend;<select|deselect>[;id] picks a friend or drops
// it (an absent id picks 0), _friend;delall clears the friends list,
// _friend;delconfirm asks to, _friend;del removes the picked friends and
// _friend;mail opens the mail form to them; _block;<action> does the same
// for the block list and shows it whatever the action.
func (l *GameClientLink) boardFriends(live *livePlayer, command string) {
	switch {
	case strings.HasPrefix(command, "_friendlist"):
		l.showFriendList(live, false)
	case strings.HasPrefix(command, "_blocklist"):
		l.showBlockList(live, false)
	case strings.HasPrefix(command, "_friend"):
		action, id, ok := boardSelection(command)
		if !ok {
			return
		}
		l.boardFriendAction(live, action, id)
	default:
		action, id, ok := boardSelection(command)
		if !ok {
			return
		}
		l.boardBlockAction(live, action, id)
	}
}

// boardSelection reads a friends board command's action and the id it
// names, 0 when it names none; false when it has no action or its id does
// not read.
func boardSelection(command string) (string, int32, bool) {
	tokens := bbs.Tokens(command, ";")
	if len(tokens) < 2 {
		return "", 0, false
	}
	if len(tokens) < 3 {
		return tokens[1], 0, true
	}
	if tokens[1] != "select" && tokens[1] != "deselect" {
		return tokens[1], 0, true
	}
	id, err := commons.ParseInt(tokens[2], 32)
	if err != nil {
		return "", 0, false
	}
	return tokens[1], int32(id), true
}

func (l *GameClientLink) boardFriendAction(live *livePlayer, action string, id int32) {
	session := &live.board
	switch action {
	case "select":
		session.friends.Add(id)
		l.showFriendList(live, false)
	case "deselect":
		session.friends.Remove(id)
		l.showFriendList(live, false)
	case "delall":
		friends := l.relations.FriendIDs(live.ObjectID())
		names := l.characterNames(friends)
		for _, friendID := range friends {
			if l.relations.RemoveFriend(live.ObjectID(), friendID) {
				live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, names[friendID]))
			}
		}
		session.friends.Clear()
		l.showFriendList(live, false)
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, friendsCleared))
		live.SendFrame(serverpackets.FrameFriendList(l.friendListEntries(live.ObjectID())))
	case "delconfirm":
		l.showFriendList(live, true)
	case "del":
		picked := session.friends.IDs()
		names := l.characterNames(picked)
		for _, friendID := range picked {
			// An online friend is the one told: by the time live's own
			// removal runs the two are no longer friends.
			if friend, online := l.livePlayerByID(friendID); online {
				if l.relations.RemoveFriend(friendID, live.ObjectID()) {
					friend.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, live.Name))
				}
				friend.SendFrame(serverpackets.FrameFriendList(l.friendListEntries(friendID)))
			}
			if l.relations.RemoveFriend(live.ObjectID(), friendID) {
				live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, names[friendID]))
			}
		}
		session.friends.Clear()
		l.showFriendList(live, false)
		live.SendFrame(serverpackets.FrameFriendList(l.friendListEntries(live.ObjectID())))
	case "mail":
		if !session.friends.Empty() {
			l.showFriendMail(live)
		}
	}
}

func (l *GameClientLink) boardBlockAction(live *livePlayer, action string, id int32) {
	session := &live.board
	confirm := false
	switch action {
	case "select":
		session.blocks.Add(id)
	case "deselect":
		session.blocks.Remove(id)
	case "delall":
		l.unblockAll(live, l.relations.BlockedIDs(live.ObjectID()))
		session.blocks.Clear()
	case "delconfirm":
		confirm = true
	case "del":
		l.unblockAll(live, session.blocks.IDs())
		session.blocks.Clear()
	}
	l.showBlockList(live, confirm)
}

// unblockAll takes ids off live's block list, telling live of each one
// that was on it.
func (l *GameClientLink) unblockAll(live *livePlayer, ids []int32) {
	names := l.characterNames(ids)
	for _, id := range ids {
		if l.relations.Unblock(live.ObjectID(), id) {
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1RemovedFromYourIgnoreList, names[id]))
		}
	}
}

// boardFriendsWrite submits the friends mail form: mail (recipients, and
// subject and message in the fourth and fifth arguments).
func (l *GameClientLink) boardFriendsWrite(live *livePlayer, args [5]string) {
	if !strings.EqualFold(args[0], "mail") {
		l.sendBoard(live, bbs.NotImplemented(args[0]))
		return
	}
	if l.board.mail == nil {
		return
	}
	l.sendMail(live, args[1], args[3], args[4])
	l.showFriendList(live, false)
}

func (l *GameClientLink) showFriendList(live *livePlayer, confirm bool) {
	page, ok := l.boardPage(bbs.FriendListPage)
	if !ok {
		return
	}
	friends := l.relations.FriendIDs(live.ObjectID())
	picked := live.board.friends.IDs()
	l.sendBoard(live, bbs.RenderFriendList(page, friends, picked, l.boardContacts(friends, picked), confirm))
}

func (l *GameClientLink) showBlockList(live *livePlayer, confirm bool) {
	page, ok := l.boardPage(bbs.BlockListPage)
	if !ok {
		return
	}
	blocked := l.relations.BlockedIDs(live.ObjectID())
	picked := live.board.blocks.IDs()
	l.sendBoard(live, bbs.RenderBlockList(page, blocked, picked, l.boardContacts(blocked, picked), confirm))
}

func (l *GameClientLink) showFriendMail(live *livePlayer) {
	page, ok := l.boardPage(bbs.FriendMailPage)
	if !ok {
		return
	}
	picked := live.board.friends.IDs()
	l.sendBoard(live, bbs.RenderFriendMail(page, picked, l.boardContacts(picked)))
}
