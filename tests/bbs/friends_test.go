package bbs

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// contact is one row of a friends board list.
func contact(command, action, label string, id int32, name string, online bool) string {
	state := "(off)"
	if online {
		state = "(on)"
	}
	return `<a action="bypass ` + command + ";" + action + ";" + strconv.Itoa(int(id)) + `">` + label + "</a>&nbsp;" + name + " " + state + "<br1>"
}

const friendDeleteAll = "<br>\n<table><tr><td width=10></td><td>Are you sure you want to delete all friends from your Friends List?</td><td width=20></td><td><button value=\"OK\" action=\"bypass _friend;delall\" back=\"l2ui_ch3.smallbutton2_down\" width=65 height=20 fore=\"l2ui_ch3.smallbutton2\"></td></tr></table>"

// TestFriendsBoard walks the friends board: the list shows each friend
// online or not; selecting moves one to the picked list; the mail form
// names the picked; del removes the picked friend on both sides, the
// online friend being told and sent its list; delconfirm adds the
// delete-all question.
func TestFriendsBoard(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	carolID := p.srv.SeedCharacterFor(t, "player3", "Carol", 1, 0).ID
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	p.srv.Relations.AddFriend(p.aliceID, carolID)
	p.enterAll(t)

	page := pageOf(t, command(t, p.alice, "_friendlist_0_"))
	bob := contact("_friend", "select", "[Select]", p.bobbyID, "Bobby", true)
	carol := contact("_friend", "select", "[Select]", carolID, "Carol", false)
	if page != "FRIENDS list="+bob+carol+" picked= del=\n" && page != "FRIENDS list="+carol+bob+" picked= del=\n" {
		t.Fatalf("friends list = %q", page)
	}

	page = pageOf(t, command(t, p.alice, "_friend;select;"+strconv.Itoa(int(p.bobbyID))))
	if want := "FRIENDS list=" + carol + " picked=" + contact("_friend", "deselect", "[Deselect]", p.bobbyID, "Bobby", true) + " del=\n"; page != want {
		t.Fatalf("friends list with Bobby picked = %q, want %q", page, want)
	}
	if got := pageOf(t, command(t, p.alice, "_friend;mail")); got != "FRIENDMAIL Bobby\n" {
		t.Fatalf("friend mail form = %q", got)
	}
	if got := pageOf(t, command(t, p.alice, "_friend;delconfirm")); got != "FRIENDS list="+carol+" picked="+contact("_friend", "deselect", "[Deselect]", p.bobbyID, "Bobby", true)+" del="+friendDeleteAll+"\n" {
		t.Fatalf("delete confirmation page = %q", got)
	}

	frames := command(t, p.alice, "_friend;del")
	bobby := drainFrames(t, p.bobby)
	if len(bobby) != 2 {
		t.Fatalf("removed online friend got %x, want its message and FriendList", opcodes(bobby))
	}
	assertSystemMessage(t, bobby[0], serverpackets.SystemMessageS1DeletedFromFriendsList, "Alice")
	assertOpcode(t, bobby[1], serverpackets.OpcodeFriendList, "FriendList")
	// The online friend was the one told: Alice gets her page and list.
	if len(frames) != 4 {
		t.Fatalf("del answer = %x, want the list page and FriendList", opcodes(frames))
	}
	if got := pageOf(t, frames); got != "FRIENDS list="+carol+" picked= del=\n" {
		t.Fatalf("list after del = %q", got)
	}
	assertOpcode(t, frames[3], serverpackets.OpcodeFriendList, "FriendList")
	if p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) {
		t.Fatal("Alice and Bobby are still friends after del")
	}

	// Deleting every friend tells Alice of each offline one, then shows
	// the emptied list, the clearing message and FriendList.
	frames = command(t, p.alice, "_friend;delall")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageS1DeletedFromFriendsList, "Carol")
	if got := pageOf(t, frames[1:]); got != "FRIENDS list= picked= del=\n" {
		t.Fatalf("list after delall = %q", got)
	}
	assertSystemMessage(t, frames[4], serverpackets.SystemMessageS1, "You have cleared your friends list.")
	assertOpcode(t, frames[5], serverpackets.OpcodeFriendList, "FriendList")
	if len(frames) != 6 {
		t.Fatalf("delall answer = %x", opcodes(frames))
	}
}

// TestBlockBoard walks the block list: select, del (with
// S1_WAS_REMOVED_FROM_YOUR_IGNORE_LIST) and an unknown action, which still
// shows the list.
func TestBlockBoard(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)
	p.alice.Send(encodeBlock(clientpackets.BlockAdd, "Bobby"))
	drainFrames(t, p.alice)
	drainFrames(t, p.bobby)

	blocked := contact("_block", "select", "[Select]", p.bobbyID, "Bobby", true)
	if got := pageOf(t, command(t, p.alice, "_blocklist_0_")); got != "BLOCKS list="+blocked+" picked= del=\n" {
		t.Fatalf("block list = %q", got)
	}
	if got := pageOf(t, command(t, p.alice, "_block;bogus")); got != "BLOCKS list="+blocked+" picked= del=\n" {
		t.Fatalf("block list after an unknown action = %q", got)
	}
	command(t, p.alice, "_block;select;"+strconv.Itoa(int(p.bobbyID)))
	frames := command(t, p.alice, "_block;del")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageS1RemovedFromYourIgnoreList, "Bobby")
	if got := pageOf(t, frames[1:]); got != "BLOCKS list= picked= del=\n" {
		t.Fatalf("block list after del = %q", got)
	}
	if p.srv.Relations.IsBlocked(p.aliceID, p.bobbyID) {
		t.Fatal("Bobby is still blocked after del")
	}
	if frames := command(t, p.alice, "_block"); len(frames) != 0 {
		t.Fatalf("_block with no action answer = %x, want silence", opcodes(frames))
	}
}

// TestFriendsBoardMail sends the friends mail form's mail to its list.
func TestFriendsBoardMail(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	p.enterAll(t)

	frames := write(t, p.alice, "_friend", "mail", "Bobby", "Title", "Hi", "Message")
	assertNewMail(t, drainFrames(t, p.bobby))
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageSentMail)
	if got := pageOf(t, frames[1:]); got != "FRIENDS list="+contact("_friend", "select", "[Select]", p.bobbyID, "Bobby", true)+" picked= del=\n" {
		t.Fatalf("friends list after mail = %q", got)
	}
	if rows := mailRows(t, p.srv); len(rows) != 2 || rows[0].subject != "Hi" || rows[0].message != "Message" {
		t.Fatalf("bbs_mail rows = %+v, want subject Hi, message Message", rows)
	}
}
