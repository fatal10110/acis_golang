package network

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// friendsCleared is the message clearing the friends list on the board
// answers with.
const friendsCleared = "You have cleared your friends list."

// boardContacts names the characters a friends board page lists, from
// names read once per page (withNames).
type boardContacts struct {
	link  *GameClientLink
	names map[int32]string
}

// withNames runs fn on live's queue with the stored names of ids. The names
// are read off the queue, on the connection goroutine of the request
// running (deferLookup); fn follows whatever that request deferred before.
func (l *GameClientLink) withNames(live *livePlayer, ids []int32, fn func(names map[int32]string)) {
	if len(ids) == 0 || l.characters == nil {
		live.afterLookups(func() { fn(nil) })
		return
	}
	live.deferLookup(func(ctx context.Context) {
		names := l.characterNames(ctx, ids)
		onLive(live, func() { fn(names) })
	})
}

// sendFriendListTo sends to the FriendList of to's friends, their names
// read off live's queue (withNames).
func (l *GameClientLink) sendFriendListTo(live, to *livePlayer) {
	ids := l.friendIDs(to.ObjectID())
	l.withNames(live, ids, func(names map[int32]string) {
		to.SendFrame(serverpackets.FrameFriendList(l.friendEntries(ids, names)))
	})
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
	id, err := strconv.ParseInt(tokens[2], 10, 32)
	if err != nil {
		return "", 0, false
	}
	return tokens[1], int32(id), true
}

func (l *GameClientLink) boardFriendAction(live *livePlayer, action string, id int32) {
	sim.AssertOwner(live.Queue())
	session := &live.board
	switch action {
	case "select":
		// #5102: only a friend on live's own list can be picked, and never
		// more of them than the list holds; another id still shows the
		// page, naming nothing of it.
		if friends := l.relations.FriendIDs(live.ObjectID()); slices.Contains(friends, id) {
			session.friends.Add(id, len(friends))
		}
		l.showFriendList(live, false)
	case "deselect":
		session.friends.Remove(id)
		l.showFriendList(live, false)
	case "delall":
		friends := l.relations.FriendIDs(live.ObjectID())
		l.withNames(live, friends, func(names map[int32]string) {
			for _, friendID := range friends {
				if l.relations.RemoveFriend(live.ObjectID(), friendID) {
					live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, names[friendID]))
				}
			}
			session.friends.Clear()
			l.showFriendList(live, false)
			live.afterLookups(func() {
				live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, friendsCleared))
			})
			l.sendFriendListTo(live, live)
		})
	case "delconfirm":
		l.showFriendList(live, true)
	case "del":
		picked := session.friends.IDs()
		l.withNames(live, picked, func(names map[int32]string) {
			for _, friendID := range picked {
				// An online friend is the one told: by the time live's own
				// removal runs the two are no longer friends.
				if friend, online := l.livePlayerByID(friendID); online {
					if l.relations.RemoveFriend(friendID, live.ObjectID()) {
						friend.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, live.CharacterName()))
					}
					l.sendFriendListTo(live, friend)
				}
				if l.relations.RemoveFriend(live.ObjectID(), friendID) {
					live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeletedFromFriendsList, names[friendID]))
				}
			}
			session.friends.Clear()
			l.showFriendList(live, false)
			l.sendFriendListTo(live, live)
		})
	case "mail":
		if !session.friends.Empty() {
			l.showFriendMail(live)
		}
	}
}

func (l *GameClientLink) boardBlockAction(live *livePlayer, action string, id int32) {
	sim.AssertOwner(live.Queue())
	session := &live.board
	confirm := false
	switch action {
	case "select":
		// #5102: as for friends, only an id on live's own block list.
		if blocked := l.relations.BlockedIDs(live.ObjectID()); slices.Contains(blocked, id) {
			session.blocks.Add(id, len(blocked))
		}
	case "deselect":
		session.blocks.Remove(id)
	case "delall":
		l.unblockAll(live, l.relations.BlockedIDs(live.ObjectID()))
		return
	case "delconfirm":
		confirm = true
	case "del":
		l.unblockAll(live, session.blocks.IDs())
		return
	}
	l.showBlockList(live, confirm)
}

// unblockAll takes ids off live's block list, telling live of each one
// that was on it, then clears the board's picks and shows the block list.
func (l *GameClientLink) unblockAll(live *livePlayer, ids []int32) {
	l.withNames(live, ids, func(names map[int32]string) {
		for _, id := range ids {
			if l.relations.Unblock(live.ObjectID(), id) {
				live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1RemovedFromYourIgnoreList, names[id]))
			}
		}
		live.board.blocks.Clear()
		l.showBlockList(live, false)
	})
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
	l.sendMail(live, args[1], args[3], args[4], func() { l.showFriendList(live, false) })
}

func (l *GameClientLink) showFriendList(live *livePlayer, confirm bool) {
	page, ok := l.boardPage(bbs.FriendListPage)
	if !ok {
		return
	}
	friends := l.relations.FriendIDs(live.ObjectID())
	picked := live.board.friends.IDs()
	l.withNames(live, withPicked(friends, picked), func(names map[int32]string) {
		l.sendBoard(live, bbs.RenderFriendList(page, friends, picked, boardContacts{link: l, names: names}, confirm))
	})
}

func (l *GameClientLink) showBlockList(live *livePlayer, confirm bool) {
	page, ok := l.boardPage(bbs.BlockListPage)
	if !ok {
		return
	}
	blocked := l.relations.BlockedIDs(live.ObjectID())
	picked := live.board.blocks.IDs()
	l.withNames(live, withPicked(blocked, picked), func(names map[int32]string) {
		l.sendBoard(live, bbs.RenderBlockList(page, blocked, picked, boardContacts{link: l, names: names}, confirm))
	})
}

// withPicked is listed and the picked ids not on it: each id a list page
// names, once.
func withPicked(listed, picked []int32) []int32 {
	ids := slices.Clone(listed)
	on := make(map[int32]struct{}, len(listed))
	for _, id := range listed {
		on[id] = struct{}{}
	}
	for _, id := range picked {
		if _, ok := on[id]; !ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (l *GameClientLink) showFriendMail(live *livePlayer) {
	page, ok := l.boardPage(bbs.FriendMailPage)
	if !ok {
		return
	}
	picked := live.board.friends.IDs()
	l.withNames(live, picked, func(names map[int32]string) {
		l.sendBoard(live, bbs.RenderFriendMail(page, picked, boardContacts{link: l, names: names}))
	})
}
