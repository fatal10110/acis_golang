package bbs

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// namesQuery is how the reads of stored characters by id and by name start.
const namesQuery = "SELECT obj_Id, char_name"

// The board's character reads below are made while the characters table is
// held. While a read waits, the player's queue still runs work posted to
// it; once the lock lifts the answer arrives as without the hold. Were the
// read made on the queue, the probe would wait behind it.

// TestFriendsBoardDelAllHoldsNoQueue clears the friends list from the board
// under the hold: the removed friend's message, the emptied list, the
// clearing message and FriendList keep their order.
func TestFriendsBoardDelAllHoldsNoQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	carolID := p.srv.SeedCharacterFor(t, "player3", "Carol", 1, 0).ID
	p.srv.Relations.AddFriend(p.aliceID, carolID)
	p.enterAll(t)

	release := p.srv.LockTable(t, "characters")
	p.alice.Send(encodeBypass("_friend;delall"))
	p.srv.AwaitLockWait(t, namesQuery)
	p.srv.AwaitQueueRuns(t, p.aliceID)
	release()

	frames := drainFrames(t, p.alice)
	if len(frames) != 6 {
		t.Fatalf("delall answer = %x", opcodes(frames))
	}
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageS1DeletedFromFriendsList, "Carol")
	if got := pageOf(t, frames[1:]); got != "FRIENDS list= picked= del=\n" {
		t.Fatalf("list after delall = %q", got)
	}
	assertSystemMessage(t, frames[4], serverpackets.SystemMessageS1, "You have cleared your friends list.")
	assertOpcode(t, frames[5], serverpackets.OpcodeFriendList, "FriendList")
}

// TestMailSendHoldsNoQueue sends a mail under the hold, its recipient
// looked up by name: the recipient is told of it, and the sender gets
// SENT_MAIL then its sent box naming the sender, once the lock lifts.
func TestMailSendHoldsNoQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	before := time.Now()
	release := p.srv.LockTable(t, "characters")
	p.alice.Send(encodeWrite("Mail", "Send", "%postId%", "Bobby", "Hello", "text"))
	p.srv.AwaitLockWait(t, namesQuery)
	p.srv.AwaitQueueRuns(t, p.aliceID)
	release()

	alice := drainFrames(t, p.alice)
	assertNewMail(t, drainFrames(t, p.bobby))
	if len(alice) != 4 {
		t.Fatalf("sender frames = %x, want SENT_MAIL then the sent box", opcodes(alice))
	}
	assertSystemMessage(t, alice[0], serverpackets.SystemMessageSentMail)
	rows := mailRows(t, p.srv)
	if len(rows) != 2 {
		t.Fatalf("bbs_mail rows = %+v, want the inbox copy and the sent copy", rows)
	}
	want := mailListPage(0, 1, 0, 0, "SENTBOX", mailRowHTML("Alice", rows[1].id, "Hello", false, before.Format("2006-01-02 15:04")), onePagePager("SENTBOX"))
	if got := pageOf(t, alice[1:]); got != want {
		t.Fatalf("sent box page =\n%q\nwant\n%q", got, want)
	}
}

// TestMailViewHoldsNoQueue opens a mail under the hold, its sender's name
// read from the database.
func TestMailViewHoldsNoQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)
	write(t, p.alice, "Mail", "Send", "%postId%", "Bobby", "Hello", "text")
	drainFrames(t, p.bobby)
	inbox := mailRows(t, p.srv)[0]

	release := p.srv.LockTable(t, "characters")
	p.bobby.Send(encodeBypass("_bbsmail;view;" + strconv.Itoa(int(inbox.id))))
	p.srv.AwaitLockWait(t, namesQuery)
	p.srv.AwaitQueueRuns(t, p.bobbyID)
	release()

	frames := drainFrames(t, p.bobby)
	view := pageOf(t, frames)
	if want := " writer=Alice "; !strings.Contains(view, want) {
		t.Fatalf("mail page = %q, want %s", view, want)
	}
}
