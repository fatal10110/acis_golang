package bbs

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// seededClanID is the clan the login tests seed Alice into.
const seededClanID = 268435456

// seedClan makes Alice the leader of a level 2 clan whose notice is set,
// shown at login or not.
func seedClan(t *testing.T, noticeShown bool, notice string) gameservertest.Option {
	return seedClanAt(t, 2, noticeShown, notice)
}

// seedClanAt is seedClan with the clan at level.
func seedClanAt(t *testing.T, level int, noticeShown bool, notice string) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{`UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Alice'`, []any{seededClanID}},
			{`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, enabled, notice, introduction)
				SELECT ?, 'Wolves', ?, obj_Id, ?, ?, 'We hunt.' FROM characters WHERE char_name = 'Alice'`, []any{seededClanID, level, noticeShown, notice}},
		} {
			if _, err := db.ExecContext(context.Background(), q.query, q.args...); err != nil {
				t.Fatalf("%s: %v", q.query, err)
			}
		}
	})
}

// seedUnreadMail files one unread mail from Alice in Alice's inbox.
func seedUnreadMail(t *testing.T) gameservertest.Option {
	return gameservertest.WithBoardSeed(func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), `INSERT INTO bbs_mail (id, receiver_id, sender_id, location, recipients, subject, message, sent_date, is_unread)
			SELECT 1, obj_Id, obj_Id, 'inbox', 'Alice', 'note', 'text', NOW(), 1 FROM characters WHERE char_name = 'Alice'`); err != nil {
			t.Fatalf("seed mail: %v", err)
		}
	})
}

// burstTail returns the frames of a login burst after its ShortCutInit.
func burstTail(t *testing.T, burst [][]byte) [][]byte {
	t.Helper()
	for i, frame := range burst {
		if frame[0] == serverpackets.OpcodeShortCutInit {
			return burst[i+1:]
		}
	}
	t.Fatalf("login burst %x has no ShortCutInit", opcodes(burst))
	return nil
}

// assertTail asserts a login burst ends, after its ShortCutInit, with want
// opcodes in order.
func assertTail(t *testing.T, tail [][]byte, want ...byte) {
	t.Helper()
	if got := opcodes(tail); string(got) != string(want) {
		t.Fatalf("login burst after ShortCutInit = %x, want %x", got, want)
	}
}

// loginHTML reads the NpcHtmlMessage of a login burst.
func loginHTML(t *testing.T, frame []byte) string {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeNpcHtmlMessage, "NpcHtmlMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != 0 {
		t.Fatalf("NpcHtmlMessage object id = %d, want 0", id)
	}
	html := r.ReadString()
	if item := r.ReadInt32(); item != 0 {
		t.Fatalf("NpcHtmlMessage item id = %d, want 0", item)
	}
	return html
}

func bootAlone(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Alice", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(boardPages),
	}, opts...)...)
	return srv, srv.Client
}

// TestLoginNewMailThenClanNotice pins the login burst of a clan member
// with unread mail and its clan's notice shown, the board on: after
// ShortCutInit come NEW_MAIL, the mail sound and ExMailArrived, then the
// clan notice window, then SkillCoolTime and the closing ActionFailed. The
// notice's line breaks become <br> and the words action and bypass are
// taken out; the server news, though shown, gives way to it.
func TestLoginNewMailThenClanNotice(t *testing.T) {
	_, c := bootAlone(t,
		gameservertest.WithCommunityBoard(boardOn),
		gameservertest.WithServerNews(true),
		seedClan(t, true, "Raid at 9\r\n<a action=\"bypass -h x\">go</a>"),
		seedUnreadMail(t),
	)
	tail := burstTail(t, enterWorld(t, c))
	assertTail(t, tail,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound, serverpackets.OpcodeExtended,
		serverpackets.OpcodeNpcHtmlMessage,
		serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)
	assertNewMail(t, tail[:3])
	want := "<html><title>Wolves</title><body>Raid at 9<br><a =\" -h x\">go</a></body></html>\n"
	if got := loginHTML(t, tail[3]); got != want {
		t.Fatalf("clan notice = %q, want %q", got, want)
	}
}

// TestLoginServerNews pins ShowServerNews: with no clan notice shown, the
// server news window takes its place in the burst; a notice that is set
// but not shown does not count.
func TestLoginServerNews(t *testing.T) {
	_, c := bootAlone(t,
		gameservertest.WithCommunityBoard(boardOn),
		gameservertest.WithServerNews(true),
		seedClan(t, false, "hidden"),
	)
	tail := burstTail(t, enterWorld(t, c))
	assertTail(t, tail, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)
	if got, want := loginHTML(t, tail[0]), "<html><body>SERVER NEWS</body></html>\n"; got != want {
		t.Fatalf("server news = %q, want %q", got, want)
	}
}

// TestLoginBoardOff pins the board switched off: unread mail and a shown
// clan notice add nothing to the burst, and the server news shows in the
// notice's place.
func TestLoginBoardOff(t *testing.T) {
	_, c := bootAlone(t,
		gameservertest.WithServerNews(true),
		seedClan(t, true, "Raid at 9"),
		seedUnreadMail(t),
	)
	tail := burstTail(t, enterWorld(t, c))
	assertTail(t, tail, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)
	if got, want := loginHTML(t, tail[0]), "<html><body>SERVER NEWS</body></html>\n"; got != want {
		t.Fatalf("server news = %q, want %q", got, want)
	}
}

// TestLoginQuiet pins the shipped settings, board and server news off: the
// burst ends ShortCutInit, SkillCoolTime, ActionFailed.
func TestLoginQuiet(t *testing.T) {
	_, c := bootAlone(t, seedClan(t, true, "Raid at 9"), seedUnreadMail(t))
	assertTail(t, burstTail(t, enterWorld(t, c)), serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)
}

// TestLoginReadMailIsQuiet pins the new-mail notice to unread mail: once
// the mail is opened on the board, the next login adds nothing.
func TestLoginReadMailIsQuiet(t *testing.T) {
	srv, c := bootAlone(t, gameservertest.WithCommunityBoard(boardOn), seedUnreadMail(t))
	assertNewMail(t, burstTail(t, enterWorld(t, c)))
	command(t, c, "_bbsmail;view;1")

	c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	assertOpcode(t, c.Read(), serverpackets.OpcodeRestartResponse, "RestartResponse")
	id := srv.SoleObjectID(t)
	srv.AdvanceUntil(t, "Alice leaves the world", func() bool {
		_, ok := srv.State.Player(id)
		return !ok
	})
	drainFrames(t, c)
	assertTail(t, burstTail(t, enterWorld(t, c)), serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)
}
