package admin

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// raiderClanID is the Raider's clan: level 5, no castle.
const raiderClanID int32 = 0x7f000201

// siegeAdmin is a booted world of the shipped castles with their sieges
// running: the Raider leading raiderClanID and a game master, both in the
// world.
type siegeAdmin struct {
	srv      *gameservertest.Server
	gm       *testsupport.ScriptedClient
	gmID     int32
	raider   *testsupport.ScriptedClient
	raiderID int32
}

func bootSiegeAdmin(t *testing.T) *siegeAdmin {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Raider", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAdmin(shippedAdminData(t)),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "castle.htm", "castles.htm")),
		gameservertest.WithReuseDelays(3*time.Second, 0),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []string{
				"UPDATE characters SET clanid = ? WHERE char_name = 'Raider'",
				"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT ?, 'Raiders', 5, obj_Id FROM characters WHERE char_name = 'Raider'",
			} {
				if _, err := db.ExecContext(context.Background(), q, raiderClanID); err != nil {
					t.Fatalf("seed raider clan: %v", err)
				}
			}
		}),
	)
	w := &siegeAdmin{srv: srv, raider: srv.Client, raiderID: srv.SoleObjectID(t)}
	enterWorld(t, w.raider)
	w.gm, w.gmID = addPlayer(t, srv, "gm", "Admin", adminLevel)
	drain(t, w.raider)
	return w
}

// siege sends //siege args from the game master and returns its frames.
func (w *siegeAdmin) siege(t *testing.T, args string) [][]byte {
	t.Helper()
	return exchange(t, w.gm, encodeBuildCmd(strings.TrimSpace("siege "+args)))
}

// target has the game master select objectID.
func (w *siegeAdmin) target(t *testing.T, objectID int32) {
	t.Helper()
	exchange(t, w.gm, encodeAction(objectID))
	drain(t, w.raider)
}

// raiderFrames returns what the Raider received up to a barrier.
func (w *siegeAdmin) raiderFrames(t *testing.T) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, w.raider, func() { w.raider.Send(encodeManorBarrier()) }, serverpackets.OpcodeExtended)
}

// storedClans counts Gludio's siege_clans rows of type side.
func (w *siegeAdmin) storedClans(t *testing.T, side string) int64 {
	t.Helper()
	w.srv.FlushPersistence(t)
	var n int64
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM siege_clans WHERE castle_id = 1 AND type = ?", side).Scan(&n); err != nil {
		t.Fatalf("count siege clans: %v", err)
	}
	return n
}

// systemMessageID returns a SystemMessage frame's id, -1 for another
// frame.
func systemMessageID(frame []byte) int {
	if frame[0] != serverpackets.OpcodeSystemMessage {
		return -1
	}
	return int(wire.NewReader(frame[1:]).ReadInt32())
}

// castleList is the castle list the shipped castles.htm makes, each
// castle's siege at the status statuses names (REGISTRATION_OPENED
// otherwise).
func castleList(t *testing.T, statuses map[string]string) string {
	t.Helper()
	pages := shippedAdminPages(t, "castles.htm")
	raw, ok := gameservertest.HTMLCache(t, pages).Get("data/html/admin/castles.htm")
	if !ok {
		t.Fatal("no admin/castles.htm in the HTML cache")
	}
	castles := []struct{ alias, name string }{
		{"gludio_castle", "Gludio Castle"},
		{"dion_castle", "Dion Castle"},
		{"giran_castle", "Giran Castle"},
		{"oren_castle", "Oren Castle"},
		{"aden_castle", "Aden Castle"},
		{"innadrile_castle", "Innadril Castle"},
		{"godad_castle", "Goddard Castle"},
		{"rune_castle", "Rune Castle"},
		{"schuttgart_castle", "Schuttgart Castle"},
	}
	var rows strings.Builder
	for i, c := range castles {
		status, ok := statuses[c.alias]
		if !ok {
			status = "REGISTRATION_OPENED"
		}
		if i%2 == 0 {
			rows.WriteString("<table width=270 bgcolor=000000><tr>")
		} else {
			rows.WriteString("<table width=270><tr>")
		}
		rows.WriteString(`<td width=100><a action="bypass -h admin_siege ` + c.alias + `">` + c.name + `</a></td><td width=130>` + status +
			`</td><td width=40 align=right><a action="bypass admin_siege list ` + c.alias + `">View</a></td></tr></table><img src="L2UI.SquareGray" width=270 height=1>`)
	}
	return strings.ReplaceAll(raw, "%castles%", rows.String())
}

// assertCastleList requires frames to be the castle list want alone.
func assertCastleList(t *testing.T, frames [][]byte, want string) {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("frames = %x, want the castle list alone", testsupport.FrameOpcodes(frames))
	}
	if got := htmlBody(t, frames[0]); got != want {
		t.Fatalf("castle list =\n%s\nwant\n%s", got, want)
	}
}

// gludioPage is a marker of Gludio's castle page.
const gludioPage = "bypass -h admin_siege attack gludio_castle"

// TestAdminSiegeCastleList pins the castle list (admin/castles.htm): //siege
// and //castle without a castle, with an alias no castle has, or with too
// many arguments, list every castle in castles.xml order, rows alternately
// on a black background, each linking to its page and its siege window and
// showing where its siege stands. //siege <alias> opens the castle's page.
func TestAdminSiegeCastleList(t *testing.T) {
	t.Parallel()
	w := bootSiegeAdmin(t)
	want := castleList(t, nil)
	if !strings.Contains(want, "Schuttgart Castle") {
		t.Fatalf("expected list misses the last castle:\n%s", want)
	}
	for _, payload := range [][]byte{
		encodeBuildCmd("siege"),
		encodeBuildCmd("castle"),
		encodeBypass("admin_siege"),
		encodeBuildCmd("siege nowhere_castle"),
		encodeBuildCmd("siege list nowhere_castle"),
		encodeBuildCmd("castle set nowhere_castle"),
		encodeBuildCmd("siege start gludio_castle now"),
	} {
		assertCastleList(t, exchange(t, w.gm, payload), want)
	}
	assertPage(t, w.siege(t, "gludio_castle"), gludioPage)
	assertPage(t, exchange(t, w.gm, encodeBypass("admin_siege gludio_castle")), gludioPage)
}

// TestAdminSiegeRegistration pins //siege attack, defend, list, clear and an
// unknown action on Gludio Castle, held by the NPCs: with no player
// selected the game master is told the target is incorrect; the selected
// Raider's clan is registered to attack as the Raider asking would, a
// second request and defending a castle no clan holds being refused to the
// Raider (638, 649); a selected player in no clan changes nothing. Each
// then opens the castle's page. list shows the siege window alone; clear
// drops the stored registrations; an unknown action is told the usage.
func TestAdminSiegeRegistration(t *testing.T) {
	t.Parallel()
	w := bootSiegeAdmin(t)

	frames := w.siege(t, "attack gludio_castle")
	if len(frames) != 2 {
		t.Fatalf("attack with no target = %x, want the refusal and the page", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageTargetIncorrect)
	assertPage(t, frames[1:], gludioPage)
	if n := w.storedClans(t, "ATTACKER"); n != 0 {
		t.Fatalf("attackers stored with no target = %d", n)
	}

	w.target(t, w.raiderID)
	assertPage(t, w.siege(t, "attack gludio_castle"), gludioPage)
	if got := w.raiderFrames(t); len(got) != 0 {
		t.Fatalf("Raider told %x on registering", testsupport.FrameOpcodes(got))
	}
	if n := w.storedClans(t, "ATTACKER"); n != 1 {
		t.Fatalf("attackers stored = %d, want the Raider's clan", n)
	}
	if got := w.srv.Sieges.All()[0].Attackers(); len(got) != 1 || got[0].ID() != raiderClanID {
		t.Fatalf("Gludio attackers = %v, want the Raider's clan", got)
	}

	for _, tc := range []struct {
		action string
		want   int
	}{
		{"attack", siege.MsgAlreadyRequested},
		{"defend", siege.MsgDefenderSideFull},
	} {
		assertPage(t, w.siege(t, tc.action+" gludio_castle"), gludioPage)
		got := w.raiderFrames(t)
		if len(got) != 1 {
			t.Fatalf("%s refusal to the Raider = %x, want one message", tc.action, testsupport.FrameOpcodes(got))
		}
		assertStatic(t, got[0], tc.want)
	}

	frames = w.siege(t, "list gludio_castle")
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSiegeInfo {
		t.Fatalf("list = %x, want SiegeInfo alone", testsupport.FrameOpcodes(frames))
	}
	if castle := wire.NewReader(frames[0][1:]).ReadInt32(); castle != 1 {
		t.Fatalf("SiegeInfo castle = %d, want Gludio", castle)
	}

	frames = w.siege(t, "bogus gludio_castle")
	if len(frames) != 2 {
		t.Fatalf("unknown action = %x, want the usage and the page", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "Usage: //siege [attack|clear|defend|end|list|start castleName].")
	assertPage(t, frames[1:], gludioPage)

	assertPage(t, w.siege(t, "clear gludio_castle"), gludioPage)
	if n := w.storedClans(t, "ATTACKER"); n != 0 {
		t.Fatalf("attackers stored after clear = %d", n)
	}
	if got := w.srv.Sieges.All()[0].Attackers(); len(got) != 0 {
		t.Fatalf("Gludio attackers after clear = %v", got)
	}

	// The game master is in no clan.
	w.target(t, w.gmID)
	assertPage(t, w.siege(t, "attack gludio_castle"), gludioPage)
	if n := w.storedClans(t, "ATTACKER"); n != 0 {
		t.Fatalf("attackers stored for a clanless target = %d", n)
	}
}

// TestAdminSiegeStartEnd pins //siege start and end on Gludio Castle: with
// the Raider's clan attacking, start puts the siege under way, the game
// master hearing it start before the castle's page, and the castle list
// shows it IN_PROGRESS; end ends it, a draw, the game master told before
// the page, and drops the registrations. Ending a siege not under way only
// opens the page; starting one with no attacker calls it off, announcing
// the next date.
func TestAdminSiegeStartEnd(t *testing.T) {
	t.Parallel()
	w := bootSiegeAdmin(t)
	gludio := w.srv.Sieges.All()[0]
	w.target(t, w.raiderID)
	assertPage(t, w.siege(t, "attack gludio_castle"), gludioPage)

	// told returns the system message ids among frames and requires the
	// castle's page to follow every one of them.
	told := func(frames [][]byte) []int {
		t.Helper()
		var ids []int
		page := -1
		for i, f := range frames {
			if id := systemMessageID(f); id >= 0 {
				if page >= 0 {
					t.Fatalf("message %d after the page among %x", id, testsupport.FrameOpcodes(frames))
				}
				ids = append(ids, id)
			}
			if f[0] == serverpackets.OpcodeNpcHtmlMessage {
				if !strings.Contains(htmlBody(t, f), gludioPage) {
					t.Fatalf("page %d is not Gludio's", i)
				}
				page = i
			}
		}
		if page < 0 {
			t.Fatalf("no castle page among %x", testsupport.FrameOpcodes(frames))
		}
		return ids
	}

	if got := told(w.siege(t, "start gludio_castle")); !slices.Contains(got, siege.MsgSiegeStarted) {
		t.Fatalf("start told the game master %v, want 711", got)
	}
	if !gludio.InProgress() {
		t.Fatal("siege not under way after start")
	}
	drain(t, w.gm)
	drain(t, w.raider)
	assertCastleList(t, w.siege(t, ""), castleList(t, map[string]string{"gludio_castle": "IN_PROGRESS"}))

	got := told(w.siege(t, "end gludio_castle"))
	if !slices.Contains(got, siege.MsgSiegeEnded) || !slices.Contains(got, siege.MsgSiegeDraw) {
		t.Fatalf("end told the game master %v, want 712 and 856", got)
	}
	if st := gludio.Status(); st != siege.StatusRegistrationOpened {
		t.Fatalf("status after end = %v", st)
	}
	if n := w.storedClans(t, "ATTACKER"); n != 0 {
		t.Fatalf("attackers stored after the end = %d", n)
	}
	drain(t, w.gm)
	drain(t, w.raider)

	assertPage(t, w.siege(t, "end gludio_castle"), gludioPage)

	got = told(w.siege(t, "start gludio_castle"))
	if !slices.Equal(got, []int{siege.MsgSiegeCanceledNoInterest, siege.MsgAnnouncedSiegeTime}) {
		t.Fatalf("start with no attacker told %v, want 846 then 292", got)
	}
	if gludio.InProgress() {
		t.Fatal("siege with no attacker under way")
	}
}
