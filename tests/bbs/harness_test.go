package bbs

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	gamebbs "github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const silenceWindow = 300 * time.Millisecond

// boardOn is the community board switched on, opening on its home page.
var boardOn = gamebbs.Config{Enabled: true, Home: "_bbshome"}

// The board pages the suites serve: each names its placeholders so a test
// reads back exactly what replaced them. The page cache ends every page
// with a line break.
var boardPages = map[string]string{
	"CommunityBoard/top/index.htm":                "HOME",
	"CommunityBoard/top/news.htm":                 "NEWS",
	"CommunityBoard/mail/mail.htm":                "LIST in=%inbox% sent=%sentbox% arc=%archive% tmp=%temparchive% type=%type% htype=%htype% ROWS=%maillist% PAGER=%maillistlength%",
	"CommunityBoard/mail/mail-show.htm":           "SHOW link=%maillink% writer=%writer% date=%sentDate% to=%receiver% del=%delDate% title=%title% mes=%mes% id=%mailId%",
	"CommunityBoard/mail/mail-write.htm":          "WRITE",
	"CommunityBoard/mail/mail-reply.htm":          "REPLY link=%maillink% to=%recipients% id=%mailId%",
	"CommunityBoard/clan/clanlist.htm":            "CLANS bar=%homebar% list=%clanlist%",
	"CommunityBoard/clan/clanhome.htm":            "VISITOR %clanid% %clanName% %clanLvL% %clanMembers% %clanLeader% ally=%allyName% intro=%clanIntro%",
	"CommunityBoard/clan/clanhome-leader.htm":     "LEADER %clanid% %clanName% %clanLvL% %clanMembers% %clanLeader% ally=%allyName% intro=%clanIntro%",
	"CommunityBoard/clan/clanhome-member.htm":     "MEMBER %clanid% %clanName%",
	"CommunityBoard/clan/clanhome-notice.htm":     "NOTICE %clanid% enabled=%enabled% flag=%flag%",
	"CommunityBoard/clan/clanhome-management.htm": "MANAGE %clanid% %curAnnNonPer%/%curAnnMemPer%/%curCbbNonPer%/%curCbbMemPer%",
	"CommunityBoard/clan/clanhome-mail.htm":       "CLANMAIL %clanid% %clanName%",
	"CommunityBoard/friend/friend-list.htm":       "FRIENDS list=%friendslist% picked=%selectedFriendsList% del=%deleteMSG%",
	"CommunityBoard/friend/friend-blocklist.htm":  "BLOCKS list=%blocklist% picked=%selectedBlocksList% del=%deleteMSG%",
	"CommunityBoard/friend/friend-mail.htm":       "FRIENDMAIL %list%",
	"clan_notice.htm":                             "<html><title>%clan_name%</title><body>%notice_text%</body></html>",
	"servnews.htm":                                "<html><body>SERVER NEWS</body></html>",
	"CommunityBoard/region/castlelist.htm":        "CASTLES %castleList%",
	"CommunityBoard/favorite/favorite-get.htm":    "FAVORITES <?FAV_LIST?>",
}

// pair is a booted server with two dialed clients, Alice (the primary
// account) and Bobby, seeded but not yet in the world.
type pair struct {
	srv     *gameservertest.Server
	alice   *testsupport.ScriptedClient
	aliceID int32
	bobby   *testsupport.ScriptedClient
	bobbyID int32
}

func bootPair(t *testing.T, opts ...gameservertest.Option) *pair {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Alice", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(boardPages),
	}, opts...)...)
	p := &pair{
		srv:     srv,
		alice:   srv.Client,
		aliceID: srv.SoleObjectID(t),
		bobbyID: srv.SeedCharacterFor(t, "player2", "Bobby", 1, 0).ID,
	}
	p.bobby = srv.DialClient(t, "player2", 1)
	return p
}

// enterAll brings both players into the world and drains the mutual
// spawn noise.
func (p *pair) enterAll(t *testing.T) {
	t.Helper()
	enterWorld(t, p.alice)
	enterWorld(t, p.bobby)
	drainFrames(t, p.alice)
	drainFrames(t, p.bobby)
}

// enterWorld selects the first character and enters the world, returning
// the whole login burst up to the server going quiet.
func enterWorld(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	assertOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	return drainFrames(t, c)
}

func (p *pair) setAccessLevel(t *testing.T, objID int32, level int) {
	t.Helper()
	exec(t, p.srv, "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", level, objID)
}

func exec(t *testing.T, srv *gameservertest.Server, query string, args ...any) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeBlock(typ int32, name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBlock)
	w.WriteInt32(typ)
	if typ == clientpackets.BlockAdd || typ == clientpackets.BlockRemove {
		w.WriteString(name)
	}
	return w.Bytes()
}

func encodeShowBoard() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShowBoard)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeWrite(url string, args ...string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBBSWrite)
	w.WriteString(url)
	for i := range 5 {
		arg := ""
		if i < len(args) {
			arg = args[i]
		}
		w.WriteString(arg)
	}
	return w.Bytes()
}

// command sends a board link's command and returns the answer.
func command(t *testing.T, c *testsupport.ScriptedClient, cmd string) [][]byte {
	t.Helper()
	c.Send(encodeBypass(cmd))
	return drainFrames(t, c)
}

// write submits a board form and returns the answer.
func write(t *testing.T, c *testsupport.ScriptedClient, url string, args ...string) [][]byte {
	t.Helper()
	c.Send(encodeWrite(url, args...))
	return drainFrames(t, c)
}

// drainFrames collects every frame c receives until the server goes quiet.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 400 {
		frame := c.ReadWithTimeout(silenceWindow)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 400 drains")
	return nil
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}

func assertOpcode(t *testing.T, frame []byte, want byte, what string) {
	t.Helper()
	if frame[0] != want {
		t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
	}
}

func assertSilent(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	if frame := c.ReadWithTimeout(silenceWindow); frame != nil {
		t.Fatalf("%s: received %#x, want silence", what, frame[0])
	}
}

// boardPart reads a ShowBoard frame's content: the eight tab links are
// skipped.
func boardPart(t *testing.T, frame []byte) string {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeShowBoard, "ShowBoard")
	r := wire.NewReader(frame[1:])
	if shown := r.ReadUint8(); shown != 1 {
		t.Fatalf("ShowBoard shown = %d, want 1", shown)
	}
	for range 8 {
		r.ReadString()
	}
	return r.ReadString()
}

// assertPage asserts frames are exactly one short board page, its three
// parts 101 (want), 102 and 103 (both unused), and returns want's page.
func assertPage(t *testing.T, frames [][]byte, want string) {
	t.Helper()
	if len(frames) != 3 {
		t.Fatalf("board answer = %x, want three ShowBoard parts", opcodes(frames))
	}
	got := []string{boardPart(t, frames[0]), boardPart(t, frames[1]), boardPart(t, frames[2])}
	if got[0] != "101\b"+want || got[1] != "102\bnull" || got[2] != "103\bnull" {
		t.Fatalf("board page = %q, want [101 %q, 102 null, 103 null]", got, want)
	}
}

// pageOf reads the content of a short board page made of frames[0:3].
func pageOf(t *testing.T, frames [][]byte) string {
	t.Helper()
	if len(frames) < 3 {
		t.Fatalf("board answer = %x, want three ShowBoard parts", opcodes(frames))
	}
	part := boardPart(t, frames[0])
	if !strings.HasPrefix(part, "101\b") {
		t.Fatalf("first part = %.20q, want 101", part)
	}
	if p2, p3 := boardPart(t, frames[1]), boardPart(t, frames[2]); p2 != "102\bnull" || p3 != "103\bnull" {
		t.Fatalf("parts 102/103 = %q/%q, want null", p2, p3)
	}
	return strings.TrimPrefix(part, "101\b")
}

// assertSystemMessage asserts a SystemMessage's id and its text params.
func assertSystemMessage(t *testing.T, frame []byte, id int, texts ...string) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(id) {
		t.Fatalf("system message id = %d, want %d", got, id)
	}
	if n := r.ReadInt32(); n != int32(len(texts)) {
		t.Fatalf("system message %d params = %d, want %d", id, n, len(texts))
	}
	for _, text := range texts {
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
			t.Fatalf("system message %d param type = %d, want text", id, typ)
		}
		if got := r.ReadString(); got != text {
			t.Fatalf("system message %d text = %q, want %q", id, got, text)
		}
	}
}

// assertNewMail asserts the three frames a new mail tells its recipient:
// NEW_MAIL, the mail sound and ExMailArrived.
func assertNewMail(t *testing.T, frames [][]byte) {
	t.Helper()
	if len(frames) < 3 {
		t.Fatalf("new mail notice = %x, want three frames", opcodes(frames))
	}
	assertSystemMessage(t, frames[0], serverpackets.SystemMessageNewMail)
	assertOpcode(t, frames[1], serverpackets.OpcodePlaySound, "PlaySound")
	r := wire.NewReader(frames[1][1:])
	if typ, file := r.ReadInt32(), r.ReadString(); typ != 0 || file != "systemmsg_e.1233" {
		t.Fatalf("PlaySound = (%d, %q), want (0, systemmsg_e.1233)", typ, file)
	}
	assertOpcode(t, frames[2], serverpackets.OpcodeExtended, "ExMailArrived")
	if sub := wire.NewReader(frames[2][1:]).ReadUint16(); sub != serverpackets.OpcodeExMailArrived {
		t.Fatalf("extended sub-opcode = %#x, want ExMailArrived", sub)
	}
}

// mailRow is one bbs_mail row.
type mailRow struct {
	id, receiver, sender int32
	location             string
	recipients, subject  string
	message              string
	unread               int
}

// mailRows reads every bbs_mail row, in id order, once the queued writes
// have landed.
func mailRows(t *testing.T, srv *gameservertest.Server) []mailRow {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.DB.QueryContext(context.Background(),
		"SELECT id, receiver_id, sender_id, location, recipients, subject, message, is_unread FROM bbs_mail ORDER BY id")
	if err != nil {
		t.Fatalf("read bbs_mail: %v", err)
	}
	defer rows.Close()
	var out []mailRow
	for rows.Next() {
		var r mailRow
		if err := rows.Scan(&r.id, &r.receiver, &r.sender, &r.location, &r.recipients, &r.subject, &r.message, &r.unread); err != nil {
			t.Fatalf("scan bbs_mail: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read bbs_mail: %v", err)
	}
	return out
}
