package bbs

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// mailListPage is the folder list page the test template renders.
func mailListPage(in, sent, archive, temp int, folder, rows, pager string) string {
	desc := map[string]string{"INBOX": "Inbox", "SENTBOX": "Sent Box", "ARCHIVE": "Mail Archive", "TEMPARCHIVE": "Temporary Mail Archive"}[folder]
	return fmt.Sprintf("LIST in=%d sent=%d arc=%d tmp=%d type=%s htype=%s ROWS=%s PAGER=%s\n",
		in, sent, archive, temp, desc, strings.ToLower(folder), rows, pager)
}

// onePagePager is the folder list's page links with a single page.
func onePagePager(folder string) string {
	return `<td><table><tr><td></td></tr><tr><td><button action="bypass _bbsmail;` + folder + `;1" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16></td></tr></table></td>` +
		"<td> 1 </td>" +
		`<td><table><tr><td></td></tr><tr><td><button action="bypass _bbsmail;` + folder + `;1" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16 ></td></tr></table></td>`
}

// mailRowHTML is one row of the folder list.
func mailRowHTML(writer string, id int32, subject string, unread bool, sent string) string {
	s := `<table width=610><tr><td width=5></td><td width=150>` + writer + `</td><td width=300><a action="bypass _bbsmail;view;` + strconv.Itoa(int(id)) + `">`
	if unread {
		s += `<font color="LEVEL">` + subject + `</font>`
	} else {
		s += subject
	}
	return s + `</a></td><td width=150>` + sent + `</td><td width=5></td></tr></table><img src="L2UI.Squaregray" width=610 height=1>`
}

// TestMailSendReadAndFile walks one mail end to end. Alice sends Bobby a
// mail from the write form: Bobby, online, is told of it (NEW_MAIL, the
// mail sound, ExMailArrived), Alice is told it went out (SENT_MAIL) and
// shown her sent box. The mail is stored twice, unread in Bobby's inbox
// and read in Alice's sent box, with the line breaks of its message turned
// into <br1>. Bobby's inbox lists it unread; opening it shows it with its
// markup escaped and stores it read; reply opens the form addressed to
// Alice; store files it in the archive; del removes it and goes back to
// the list page last shown.
func TestMailSendReadAndFile(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	assertPage(t, command(t, p.alice, "_bbsmail;crea"), "WRITE\n")
	before := time.Now()
	p.alice.Send(encodeWrite("Mail", "Send", "%postId%", "bOBBY", "Hello <b>", "line one\nline two"))
	alice := drainFrames(t, p.alice)
	bobby := drainFrames(t, p.bobby)
	assertNewMail(t, bobby)
	if len(bobby) != 3 {
		t.Fatalf("recipient frames = %x, want the new-mail notice alone", opcodes(bobby))
	}
	if len(alice) != 4 {
		t.Fatalf("sender frames = %x, want SENT_MAIL then the sent box", opcodes(alice))
	}
	assertSystemMessage(t, alice[0], serverpackets.SystemMessageSentMail)
	sentText := before.Format("2006-01-02 15:04")
	rows := mailRows(t, p.srv)
	if len(rows) != 2 {
		t.Fatalf("bbs_mail rows = %+v, want the inbox copy and the sent copy", rows)
	}
	inbox, sent := rows[0], rows[1]
	wantInbox := mailRow{id: inbox.id, receiver: p.bobbyID, sender: p.aliceID, location: "inbox", recipients: "bOBBY", subject: "Hello <b>", message: "line one<br1>line two", unread: 1}
	wantSent := mailRow{id: inbox.id + 1, receiver: p.aliceID, sender: p.aliceID, location: "sentbox", recipients: "bOBBY", subject: "Hello <b>", message: "line one<br1>line two", unread: 0}
	if inbox != wantInbox || sent != wantSent {
		t.Fatalf("bbs_mail rows = %+v, want %+v and %+v", rows, wantInbox, wantSent)
	}
	if got, want := pageOf(t, alice[1:]), mailListPage(0, 1, 0, 0, "SENTBOX", mailRowHTML("Alice", sent.id, "Hello <b>", false, sentText), onePagePager("SENTBOX")); got != want {
		t.Fatalf("sent box page =\n%q\nwant\n%q", got, want)
	}

	if got, want := pageOf(t, command(t, p.bobby, "_maillist_0_1_0_")), mailListPage(1, 0, 0, 0, "INBOX", mailRowHTML("Alice", inbox.id, "Hello <b>", true, sentText), onePagePager("INBOX")); got != want {
		t.Fatalf("inbox page =\n%q\nwant\n%q", got, want)
	}

	view := pageOf(t, command(t, p.bobby, "_bbsmail;view;"+strconv.Itoa(int(inbox.id))))
	wantView := `SHOW link=<a action="bypass _bbsmail">Inbox</a>&nbsp;&gt;&nbsp;Hello <b> writer=Alice date=` + sentText +
		` to=bOBBY del=Unknown title=Hello &lt;b&gt; mes=line one&lt;br1&gt;line two id=` + strconv.Itoa(int(inbox.id)) + "\n"
	if view != wantView {
		t.Fatalf("mail page =\n%q\nwant\n%q", view, wantView)
	}
	if rows := mailRows(t, p.srv); rows[0].unread != 0 {
		t.Fatalf("opened mail is_unread = %d, want 0", rows[0].unread)
	}

	reply := command(t, p.bobby, "_bbsmail;reply;"+strconv.Itoa(int(inbox.id)))
	if len(reply) != 2 {
		t.Fatalf("reply answer = %x, want the 1001 form and its 1002 fields", opcodes(reply))
	}
	wantReply := `1001` + "\b" + `REPLY link=<a action="bypass _bbsmail">Inbox</a>&nbsp;&gt;&nbsp;<a action="bypass _bbsmail;view;` + strconv.Itoa(int(inbox.id)) + `">Hello <b></a>&nbsp;&gt;&nbsp; to=Alice id=` + strconv.Itoa(int(inbox.id)) + "\n"
	if got := boardPart(t, reply[0]); got != wantReply {
		t.Fatalf("reply form = %q, want %q", got, wantReply)
	}
	wantFields := "1002\b0 \b0 \b0 \b0 \b0 \b0 \bBobby \b" + strconv.Itoa(int(p.bobbyID)) + " \bplayer2 \b9 \bRe: Hello <b> \bRe: Hello <b> \b  \b0 \b0 \b0 \b0 \b"
	if got := boardPart(t, reply[1]); got != wantFields {
		t.Fatalf("reply fields = %q, want %q", got, wantFields)
	}

	if got, want := pageOf(t, command(t, p.bobby, "_bbsmail;store;"+strconv.Itoa(int(inbox.id)))), mailListPage(0, 0, 1, 0, "ARCHIVE", mailRowHTML("Alice", inbox.id, "Hello <b>", false, sentText), onePagePager("ARCHIVE")); got != want {
		t.Fatalf("archive page =\n%q\nwant\n%q", got, want)
	}
	if rows := mailRows(t, p.srv); rows[0].location != "archive" {
		t.Fatalf("stored mail location = %q, want archive", rows[0].location)
	}

	if got, want := pageOf(t, command(t, p.bobby, "_bbsmail;del;"+strconv.Itoa(int(inbox.id)))), mailListPage(0, 0, 0, 0, "INBOX", "", onePagePager("INBOX")); got != want {
		t.Fatalf("page after delete =\n%q\nwant\n%q", got, want)
	}
	if rows := mailRows(t, p.srv); len(rows) != 1 || rows[0].id != sent.id {
		t.Fatalf("bbs_mail rows after delete = %+v, want the sent copy alone", rows)
	}
}

// TestMailCommandsOnNoMail pins a mail command naming no mail of the
// player's, or an action no mail command takes: it goes back to the list
// page last shown. The folder list's own page links name the folder in
// upper case and so land there too. A folder command whose page does not
// read shows nothing.
func TestMailCommandsOnNoMail(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	empty := mailListPage(0, 0, 0, 0, "INBOX", "", onePagePager("INBOX"))
	for _, cmd := range []string{"_bbsmail;view;999", "_bbsmail;del", "_bbsmail;INBOX;1", "_bbsmail;temp_archive", "_bbsmail;bogus;5"} {
		if got := pageOf(t, command(t, p.alice, cmd)); got != empty {
			t.Fatalf("%s page =\n%q\nwant\n%q", cmd, got, empty)
		}
	}
	if got, want := pageOf(t, command(t, p.alice, "_bbsmail;sentbox;7")), mailListPage(0, 0, 0, 0, "SENTBOX", "", onePagePager("SENTBOX")); got != want {
		t.Fatalf("sent box page past its last = %q, want %q", got, want)
	}
	for _, cmd := range []string{"_bbsmail;inbox;x", "_bbsmail;view;x", "_bbsmail"} {
		frames := command(t, p.alice, cmd)
		if cmd == "_bbsmail" {
			if got := pageOf(t, frames); got != empty {
				t.Fatalf("_bbsmail page = %q, want %q", got, empty)
			}
			continue
		}
		if len(frames) != 0 {
			t.Fatalf("%s answer = %x, want silence", cmd, opcodes(frames))
		}
	}
}

// TestMailSendRefusals pins each recipient a mail does not reach, in list
// order, with the sender's message for it: no such character
// (INVALID_TARGET), the sender itself (INVALID_TARGET), a game master
// (CANNOT_MAIL_GM_S1), a recipient blocking everything
// (S1_BLOCKED_EVERYTHING) and one blocking the sender
// (S1_BLOCKED_YOU_CANNOT_MAIL), each naming the recipient as typed. With
// nobody reached, nothing is stored and SENT_MAIL is not sent; the sent
// box shows either way.
func TestMailSendRefusals(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	gmID := p.srv.SeedCharacterFor(t, "player3", "Gamemaster", 1, 0).ID
	p.setAccessLevel(t, gmID, 1)
	p.enterAll(t)

	p.bobby.Send(encodeBlock(clientpackets.BlockAll, ""))
	drainFrames(t, p.bobby)
	frames := write(t, p.alice, "Mail", "Send", "0", " Nobody;alice;GameMaster;bobby ", "s", "m")
	if len(frames) != 7 {
		t.Fatalf("refusal frames = %x, want four messages and the sent box", opcodes(frames))
	}
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	assertSystemMessage(t, frames[1], serverpackets.SystemMessageInvalidTarget)
	assertSystemMessage(t, frames[2], serverpackets.SystemMessageCannotMailGMS1, "GameMaster")
	assertSystemMessage(t, frames[3], serverpackets.SystemMessageS1BlockedEverything, "bobby")
	assertSilent(t, p.bobby, "a recipient blocking everything")
	if rows := mailRows(t, p.srv); len(rows) != 0 {
		t.Fatalf("bbs_mail rows = %+v, want none", rows)
	}

	p.bobby.Send(encodeBlock(clientpackets.BlockAllRelease, ""))
	drainFrames(t, p.bobby)
	p.bobby.Send(encodeBlock(clientpackets.BlockAdd, "Alice"))
	drainFrames(t, p.bobby)
	drainFrames(t, p.alice)
	frames = write(t, p.alice, "Mail", "Send", "0", "Bobby", "s", "m")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageS1BlockedYouCannotMail, "Bobby")
	assertSilent(t, p.bobby, "a recipient blocking the sender")
}

// TestMailSendGameMaster pins a game master sender: it may mail more than
// five names, a game master, and a player blocking everything.
func TestMailSendGameMaster(t *testing.T) {
	levels, err := admin.NewData([]admin.AccessLevel{
		{Level: 0, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77", AllowTransaction: true},
		{Level: 7, Name: "GM", NameColor: "FFFFFF", TitleColor: "FFFF77", IsGM: true, AllowTransaction: true},
	}, nil)
	if err != nil {
		t.Fatalf("admin.NewData: %v", err)
	}
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn), gameservertest.WithAdmin(levels))
	p.setAccessLevel(t, p.bobbyID, 7)
	gmID := p.srv.SeedCharacterFor(t, "player3", "Gamemaster", 1, 0).ID
	p.setAccessLevel(t, gmID, 7)
	p.enterAll(t)

	p.alice.Send(encodeBlock(clientpackets.BlockAll, ""))
	drainFrames(t, p.alice)
	frames := write(t, p.bobby, "Mail", "Send", "0", "Alice;Gamemaster;x1;x2;x3;x4", "s", "m")
	assertNewMail(t, drainFrames(t, p.alice))
	// Alice and the game master take it; the four unknown names do not.
	for i := range 4 {
		assertSystemMessage(t, frames[i], serverpackets.SystemMessageInvalidTarget)
	}
	assertSystemMessage(t, frames[4], serverpackets.SystemMessageSentMail)
	if rows := mailRows(t, p.srv); len(rows) != 3 {
		t.Fatalf("bbs_mail rows = %+v, want two inbox copies and the sent copy", rows)
	}
}

// TestMailSendLimits pins the refusals made before any recipient is
// tried: more than five names from a player (ONLY_FIVE_RECIPIENTS), and a
// sender whose sent box took ten mails in the last day
// (NO_MORE_MESSAGES_TODAY; a sent mail older than a day does not count).
// A full inbox of 100 mails refuses its recipient alone: MESSAGE_NOT_SENT
// to the sender, MAILBOX_FULL to the recipient.
func TestMailSendLimits(t *testing.T) {
	p := bootPair(t,
		gameservertest.WithCommunityBoard(boardOn),
		gameservertest.WithBoardSeed(func(db *sql.DB) {
			var alice int32
			if err := db.QueryRowContext(context.Background(), "SELECT obj_Id FROM characters WHERE char_name = 'Alice'").Scan(&alice); err != nil {
				t.Fatalf("find Alice: %v", err)
			}
			insert := func(id int32, location string, sent time.Time) {
				if _, err := db.ExecContext(context.Background(),
					"INSERT INTO bbs_mail (id, receiver_id, sender_id, location, recipients, subject, message, sent_date, is_unread) VALUES (?, ?, ?, ?, 'x', 's', 'm', ?, 0)",
					id, alice, alice, location, sent.Format("2006-01-02 15:04:05")); err != nil {
					t.Fatalf("seed mail %d: %v", id, err)
				}
			}
			now := time.Now()
			for i := range int32(9) {
				insert(i+1, "sentbox", now.Add(-time.Hour))
			}
			insert(10, "sentbox", now.Add(-25*time.Hour))
			for i := range int32(100) {
				insert(100+i, "inbox", now.Add(-time.Hour))
			}
		}),
	)
	p.enterAll(t)

	frames := write(t, p.bobby, "Mail", "Send", "0", "a;b;c;d;e;f", "s", "m")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageOnlyFiveRecipients)
	if len(frames) != 4 {
		t.Fatalf("six recipients answer = %x, want the refusal then the sent box", opcodes(frames))
	}

	frames = write(t, p.bobby, "Mail", "Send", "0", "Alice", "s", "m")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageMessageNotSent)
	if len(frames) != 4 {
		t.Fatalf("full inbox answer = %x, want MESSAGE_NOT_SENT then the sent box", opcodes(frames))
	}
	full := drainFrames(t, p.alice)
	if len(full) != 1 {
		t.Fatalf("recipient with a full inbox got %x, want MAILBOX_FULL alone", opcodes(full))
	}
	assertSystemMessage(t, full[0], serverpackets.SystemMessageMailboxFull)

	// Nine sent in the last day: the tenth goes out.
	frames = write(t, p.alice, "Mail", "Send", "0", "Bobby", "s", "m")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageSentMail)
	drainFrames(t, p.bobby)
	frames = write(t, p.alice, "Mail", "Send", "0", "Bobby", "s", "m")
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageNoMoreMessagesToday)
	assertSilent(t, p.bobby, "recipient of a mail over the daily limit")
	if rows := mailRows(t, p.srv); len(rows) != 112 {
		t.Fatalf("bbs_mail rows = %d, want the 110 seeded and the one mail's two copies", len(rows))
	}
}

// TestMailTooLongIsNotStored pins a message wider than its column: the
// recipient still gets the mail for the run, but nothing is stored, no
// sent-box copy is filed and SENT_MAIL is not sent.
func TestMailTooLongIsNotStored(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	p.enterAll(t)

	frames := write(t, p.alice, "Mail", "Send", "0", "Bobby", "s", strings.Repeat("m", 3001))
	assertNewMail(t, drainFrames(t, p.bobby))
	if len(frames) != 3 {
		t.Fatalf("sender answer = %x, want the sent box alone", opcodes(frames))
	}
	if got, want := pageOf(t, frames), mailListPage(0, 0, 0, 0, "SENTBOX", "", onePagePager("SENTBOX")); got != want {
		t.Fatalf("sent box = %q, want %q", got, want)
	}
	if rows := mailRows(t, p.srv); len(rows) != 0 {
		t.Fatalf("bbs_mail rows = %+v, want none", rows)
	}
	if got := pageOf(t, command(t, p.bobby, "_bbsmail")); !strings.Contains(got, "in=1 ") {
		t.Fatalf("recipient inbox = %q, want the unstored mail listed", got)
	}
}
