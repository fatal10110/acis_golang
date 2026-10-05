package npcs

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: ClanHallManagerNpc.onBypassFeedback (functions, manage,
// support, support_back, list_back; the owner gate getNpcTalkCond, the
// privilege gate validatePrivileges and revalidateDeco), Npc.teleport with
// its isTeleportAllowed override, ClanHall.updateFunction and
// ClanHallFunction.refreshFunction, and ClanHallManagerNpcAI.thinkCast.
// Black (35384) manages Moonstone Hall (22), a grade 2 auctionable hall.
// clanHallDeco.xml: fireplace (type 1) level 4 costs 1000 a day, level 10
// 7500 every 3 days; curtains (type 7) level 1 2000 every 7 days.

const (
	hallManagerID   = 35384
	hallManagerClan = 0x70000031
	hallFounderID   = 0x70000032
	hallRivalClan   = 0x70000033
	hallMoonstone   = 22
	hallDayMs       = int64(24 * time.Hour / time.Millisecond)
)

// hallManagerSetup is the world a clan hall manager scenario boots with.
type hallManagerSetup struct {
	// rank, when set, makes Talker a member of that power grade holding
	// privs, the clan led by an offline founder; otherwise Talker leads.
	rank, privs int
	// foreign has a rival clan own the hall instead of Talker's.
	foreign bool
	// adena is Talker's inventory adena.
	adena int32
	// functions are seeded clanhall_functions rows of Moonstone Hall:
	// type, level, lease, rate and end time.
	functions [][5]int64
	extra     []gameservertest.Option
}

type hallManagerWorld struct {
	*folkWorld
	manager *npc.Folk
	// bystander is a second player standing in the hall, of no clan.
	bystander *testsupport.ScriptedClient
}

// hallManagerPages are the shipped clan hall manager pages and the page
// admitting every command.
func hallManagerPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "clanHallManager")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{"test/any.htm": anyNpcPage}
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		pages["clanHallManager/"+e.Name()] = string(data)
	}
	return pages
}

// managerPage is the shipped page as the page cache holds it.
func managerPage(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "clanHallManager", name))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return page
}

// hallGrounds is Moonstone Hall's zone, around the players' spawn point.
func hallGrounds(t *testing.T) *zone.Index {
	t.Helper()
	zones := zone.NewIndex()
	form, err := zone.NewCuboid(-2000, 2000, -2000, 2000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	set := commons.NewStatSet()
	set.Set("clanHallId", strconv.Itoa(hallMoonstone))
	hz, err := zone.NewClanHall(1, form, set)
	if err != nil {
		t.Fatal(err)
	}
	zones.Add(hz)
	return zones
}

func bootHallManager(t *testing.T, s hallManagerSetup) *hallManagerWorld {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatal(err)
	}
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(hallManagerPages(t)),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithZones(hallGrounds(t)),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedHallManager(t, db, s) }),
		noBypassReuse,
	}, s.extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &hallManagerWorld{folkWorld: &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}}
	if s.adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, s.adena)
	}
	srv.SeedCharacterFor(t, "player2", "Bystander", playerLevel, 0)
	w.bystander = srv.DialClient(t, "player2", 1)
	startInWorld(t, srv, w.c)
	startInWorld(t, srv, w.bystander)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	tmpl := folkTemplate("ClanHallManagerNpc", hallManagerID)
	tmpl.Name, tmpl.MPMax = "Black", 1493
	w.manager = w.spawnFolk(t, tmpl, 60)
	drainUntilQuiet(t, w.bystander)
	return w
}

func seedHallManager(t *testing.T, db *sql.DB, s hallManagerSetup) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed clan hall manager: %s: %v", q, err)
		}
	}
	if s.rank == 0 {
		exec("UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Talker'", hallManagerClan)
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
			SELECT ?, 'Hallkeepers', 4, obj_Id FROM characters WHERE char_name = 'Talker'`, hallManagerClan)
	} else {
		exec("UPDATE characters SET clanid = ?, power_grade = ? WHERE char_name = 'Talker'", hallManagerClan, s.rank)
		exec(`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('founder', ?, 'Founder', 40, ?, 0)`,
			hallFounderID, hallManagerClan)
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Hallkeepers', 4, ?)", hallManagerClan, hallFounderID)
		exec("INSERT INTO clan_privs (clan_id, ranking, privs) VALUES (?, ?, ?)", hallManagerClan, s.rank, s.privs)
	}
	owner := hallManagerClan
	if s.foreign {
		owner = hallRivalClan
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Rivals', 4)", hallRivalClan)
	}
	exec("INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (?, ?, 1, ?)", hallMoonstone, owner, time.Now().UnixMilli()+30*hallDayMs)
	for _, f := range s.functions {
		exec("INSERT INTO clanhall_functions (hall_id, type, lvl, lease, rate, endTime) VALUES (?, ?, ?, ?, ?, ?)",
			hallMoonstone, f[0], f[1], f[2], f[3], f[4])
	}
}

// run sends the manager command from the page admitting every command and
// returns the answer.
func (w *hallManagerWorld) run(t *testing.T, command string) [][]byte {
	t.Helper()
	w.openAnyNpcPage(t)
	return w.bypass(t, npcCommand(w.manager, command))
}

// page sends the manager command and returns the page it opens.
func (w *hallManagerWorld) page(t *testing.T, command string) string {
	t.Helper()
	return pageIn(t, w.run(t, command))
}

func (w *hallManagerWorld) oid() string { return strconv.Itoa(int(w.manager.ObjectID())) }

// adena is Talker's inventory adena.
func (w *hallManagerWorld) adena(t *testing.T) int {
	t.Helper()
	var n int
	w.onPlayer(t, func(pc *player.Character) { n = pc.Inventory().Adena() })
	return n
}

// storedFunctions are Moonstone Hall's clanhall_functions rows by type:
// level, lease, rate and end time.
func (w *hallManagerWorld) storedFunctions(t *testing.T) map[int][4]int64 {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.QueryContext(context.Background(),
		"SELECT type, lvl, lease, rate, endTime FROM clanhall_functions WHERE hall_id = ?", hallMoonstone)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int][4]int64{}
	for rows.Next() {
		var typ int
		var r [4]int64
		if err := rows.Scan(&typ, &r[0], &r[1], &r[2], &r[3]); err != nil {
			t.Fatal(err)
		}
		out[typ] = r
	}
	return out
}

// hallDecoration is Moonstone Hall's ClanHallDecoration with the twelve
// slot bytes.
func hallDecoration(slots ...byte) []byte {
	return append([]byte{serverpackets.OpcodeClanHallDecoration, hallMoonstone, 0, 0, 0}, slots...)
}

// decorationsIn returns the ClanHallDecoration frames among frames.
func decorationsIn(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeClanHallDecoration {
			out = append(out, f)
		}
	}
	return out
}

// answerOrder keeps the opcodes a function change is judged by, in order.
func answerOrder(frames [][]byte) []byte {
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage, serverpackets.OpcodeClanHallDecoration,
			serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed:
			order = append(order, f[0])
		}
	}
	return order
}

// TestClanHallManagerRentChangeCancel: renting HP recovery takes the fee
// from the leader's adena, stores the function for its term and shows the
// hall's new decorations to everyone inside it, before the confirmation
// page; changing it takes the new fee and restarts the term, keeping the
// first term's length; asking for the level it already has changes
// nothing; cancelling removes it for free.
func TestClanHallManagerRentChangeCancel(t *testing.T) {
	t.Parallel()
	w := bootHallManager(t, hallManagerSetup{adena: 100_000})
	oid := w.oid()

	before := w.srv.Halls.Now()
	frames := w.run(t, "manage recovery hp 4")
	after := w.srv.Halls.Now()
	want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeClanHallDecoration, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if got := answerOrder(frames); !bytes.Equal(got, want) {
		t.Fatalf("rent answer = % x, want % x", got, want)
	}
	wantMessages(t, "rent", frames, serverpackets.SystemMessageS1DisappearedAdena)
	if got := pageIn(t, frames); got != fill(managerPage(t, "functions-apply_confirmed.htm"), "%objectId%", oid) {
		t.Fatalf("rent page =\n%s", got)
	}
	// HP 80% shows fireplace level 4, depth 1.
	if got := decorationsIn(frames); len(got) != 1 || !bytes.Equal(got[0], hallDecoration(1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)) {
		t.Fatalf("decoration = % x", got)
	}
	if got := decorationsIn(drainFrames(t, w.bystander)); len(got) != 1 || !bytes.Equal(got[0], hallDecoration(1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)) {
		t.Fatalf("bystander decoration = % x", got)
	}
	if got := w.adena(t); got != 99_000 {
		t.Fatalf("adena after renting = %d, want 99000", got)
	}
	row := w.storedFunctions(t)[1]
	if row[0] != 80 || row[1] != 1000 || row[2] != hallDayMs || row[3] < before+hallDayMs || row[3] > after+hallDayMs {
		t.Fatalf("stored HP function = %v, want level 80, lease 1000, rate one day, a term from now (sent %d-%d)", row, before, after)
	}

	before = w.srv.Halls.Now()
	frames = w.run(t, "manage recovery hp 10")
	after = w.srv.Halls.Now()
	if got := pageIn(t, frames); got != fill(managerPage(t, "functions-apply_confirmed.htm"), "%objectId%", oid) {
		t.Fatalf("change page =\n%s", got)
	}
	if got := w.adena(t); got != 91_500 {
		t.Fatalf("adena after the change = %d, want 91500", got)
	}
	// Fireplace level 10 is depth 2; the term keeps its one day length.
	if got := decorationsIn(frames); len(got) != 1 || !bytes.Equal(got[0], hallDecoration(2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)) {
		t.Fatalf("decoration after the change = % x", got)
	}
	row = w.storedFunctions(t)[1]
	if row[0] != 200 || row[1] != 7500 || row[2] != hallDayMs || row[3] < before+hallDayMs || row[3] > after+hallDayMs {
		t.Fatalf("changed HP function = %v, want level 200, lease 7500, the first term's length from now", row)
	}

	frames = w.run(t, "manage recovery hp 10")
	if got := pageIn(t, frames); got != fill(managerPage(t, "functions-used.htm"), "%val%", "10%", "%objectId%", oid) {
		t.Fatalf("unchanged page =\n%s", got)
	}
	if len(decorationsIn(frames)) != 0 || w.adena(t) != 91_500 {
		t.Fatal("asking for the current level changed something")
	}

	frames = w.run(t, "manage recovery hp 0")
	if got := pageIn(t, frames); got != fill(managerPage(t, "functions-cancel_confirmed.htm"), "%objectId%", oid) {
		t.Fatalf("cancel page =\n%s", got)
	}
	wantMessages(t, "cancel", frames)
	if got := decorationsIn(frames); len(got) != 1 || !bytes.Equal(got[0], hallDecoration(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)) {
		t.Fatalf("decoration after the cancel = % x", got)
	}
	if rows := w.storedFunctions(t); len(rows) != 0 {
		t.Fatalf("stored functions after the cancel = %v, want none", rows)
	}
	if got := w.adena(t); got != 91_500 {
		t.Fatalf("adena after the cancel = %d, want 91500", got)
	}
}

// TestClanHallManagerPageFlow: the management pages' own links lead from
// the menu to a rented function.
func TestClanHallManagerPageFlow(t *testing.T) {
	t.Parallel()
	w := bootHallManager(t, hallManagerSetup{adena: 10_000})
	oid := w.oid()
	if got := w.page(t, "manage"); got != fill(managerPage(t, "manage.htm"), "%objectId%", oid) {
		t.Fatalf("menu =\n%s", got)
	}
	for _, step := range []string{"manage deco", "manage deco edit_curtains 1", "manage deco curtains 1"} {
		frames := w.bypass(t, npcCommand(w.manager, step))
		if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); !ok {
			t.Fatalf("%q opened no page (opcodes % x)", step, opcodes(frames))
		}
	}
	if row := w.storedFunctions(t)[7]; row[0] != 1 || row[1] != 2000 || row[2] != 7*hallDayMs {
		t.Fatalf("stored curtains = %v, want level 1, lease 2000, 7 days", row)
	}
}

// TestClanHallManagerLowAdena: a leader short of the fee is told so and
// shown the low adena page; nothing is rented or shown.
func TestClanHallManagerLowAdena(t *testing.T) {
	t.Parallel()
	w := bootHallManager(t, hallManagerSetup{adena: 999})
	frames := w.run(t, "manage recovery hp 4")
	wantMessages(t, "short", frames, serverpackets.SystemMessageYouNotEnoughAdena)
	if got := pageIn(t, frames); got != fill(managerPage(t, "low_adena.htm"), "%objectId%", w.oid()) {
		t.Fatalf("page =\n%s", got)
	}
	if len(decorationsIn(frames)) != 0 {
		t.Fatal("decoration shown for an unpaid rent")
	}
	if rows := w.storedFunctions(t); len(rows) != 0 {
		t.Fatalf("stored functions = %v, want none", rows)
	}
	if got := w.adena(t); got != 999 {
		t.Fatalf("adena = %d, want 999 kept", got)
	}
}

// TestClanHallManagerUnpricedLevelRefused: the grade 2 hall's "260%"
// fireplace link offers a level clanHallDeco.xml does not price; its offer
// page shows no fee, and renting it is refused instead of rented free.
func TestClanHallManagerUnpricedLevelRefused(t *testing.T) {
	t.Parallel()
	w := bootHallManager(t, hallManagerSetup{adena: 10_000})
	got := w.page(t, "manage recovery edit_hp 260")
	want := fill(managerPage(t, "functions-apply.htm"),
		"%name%", "Fireplace (HP Recovery Device)",
		"%cost%", "0</font> Adena / 0 day(s)</font>)",
		"%use%", `Provides additional HP recovery for clan members in the clan hall.<font color="00FFFF">5000%</font>`,
		"%apply%", "recovery hp 260", "%objectId%", w.oid())
	if got != want {
		t.Fatalf("offer =\n%s\nwant\n%s", got, want)
	}
	frames := w.run(t, "manage recovery hp 260")
	if got := opcodes(frames); !bytes.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("answer = % x, want ActionFailed alone", got)
	}
	if rows := w.storedFunctions(t); len(rows) != 0 {
		t.Fatalf("stored functions = %v, want none", rows)
	}
}

// TestClanHallManagerGates: only a member of the owning clan is answered;
// a member without the right to set the functions, or to use them, is
// shown the refusal page.
func TestClanHallManagerGates(t *testing.T) {
	t.Parallel()
	t.Run("foreign clan", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{foreign: true, adena: 10_000})
		for _, command := range []string{"manage", "functions", "manage recovery hp 4", "list_back"} {
			if got := opcodes(w.run(t, command)); !bytes.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
				t.Fatalf("%q answer = % x, want ActionFailed alone", command, got)
			}
		}
	})
	t.Run("functions but not their setting", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5, privs: 2048, adena: 10_000})
		if got := w.page(t, "manage recovery hp 4"); got != managerPage(t, "not_authorized.htm") {
			t.Fatalf("set functions page =\n%s", got)
		}
		if rows := w.storedFunctions(t); len(rows) != 0 {
			t.Fatalf("stored functions = %v, want none", rows)
		}
		if got := w.page(t, "functions"); !strings.Contains(got, "Hp Recovery") {
			t.Fatalf("functions page =\n%s", got)
		}
	})
	t.Run("no right", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{rank: 5})
		for _, command := range []string{"functions", "functions tele", "support_back", "teleport 0"} {
			if got := w.page(t, command); got != managerPage(t, "not_authorized.htm") {
				t.Fatalf("%q page =\n%s", command, got)
			}
		}
		// list_back needs no right.
		if got := w.page(t, "list_back"); got != fill(managerPage(t, "chamberlain.htm"), "%objectId%", w.oid()) {
			t.Fatalf("list_back page =\n%s", got)
		}
	})
}

// TestClanHallManagerFunctionPages: the recovery, other and decoration
// pages show each rented function's level, lease, term and next fee with
// the grade's change links after a removal link, and "none" with the
// links alone otherwise; the offer page names the fee and term.
func TestClanHallManagerFunctionPages(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(48 * time.Hour).UnixMilli()
	w := bootHallManager(t, hallManagerSetup{functions: [][5]int64{
		{1, 80, 1000, hallDayMs, end},
		{5, 2, 6000, 3 * hallDayMs, end},
		{11, 1, 1300, 3 * hallDayMs, end},
	}})
	oid := w.oid()
	next := "Next fee at " + time.UnixMilli(end).Format("02-01-2006 15:04")

	const (
		removeHP   = `[<a action="bypass -h npc_%objectId%_manage recovery hp_cancel">Remove</a>]`
		hpGrade2   = `[<a action="bypass -h npc_%objectId%_manage recovery edit_hp 4">80%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 7">140%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 10">200%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_hp 260">260%</a>]`
		expGrade2  = `[<a action="bypass -h npc_%objectId%_manage recovery edit_exp 1">5%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 5">25%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_exp 8">40%</a>]`
		mpGrade2   = `[<a action="bypass -h npc_%objectId%_manage recovery edit_mp 1">5%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 3">15%</a>][<a action="bypass -h npc_%objectId%_manage recovery edit_mp 6">30%</a>]`
		removeTele = `[<a action="bypass -h npc_%objectId%_manage other tele_cancel">Remove</a>]`
		tele       = `[<a action="bypass -h npc_%objectId%_manage other edit_tele 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage other edit_tele 2">Level 2</a>]`
		support2   = `[<a action="bypass -h npc_%objectId%_manage other edit_support 3">Level 3</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 4">Level 4</a>][<a action="bypass -h npc_%objectId%_manage other edit_support 5">Level 5</a>]`
		item       = `[<a action="bypass -h npc_%objectId%_manage other edit_item 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage other edit_item 2">Level 2</a>][<a action="bypass -h npc_%objectId%_manage other edit_item 3">Level 3</a>]`
		curtains   = `[<a action="bypass -h npc_%objectId%_manage deco edit_curtains 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage deco edit_curtains 2">Level 2</a>]`
		removeFix  = `[<a action="bypass -h npc_%objectId%_manage deco fixtures_cancel">Remove</a>]`
		fixtures   = `[<a action="bypass -h npc_%objectId%_manage deco edit_fixtures 1">Level 1</a>][<a action="bypass -h npc_%objectId%_manage deco edit_fixtures 2">Level 2</a>]`
	)

	want := fill(managerPage(t, "edit_recovery.htm"),
		"%hp_recovery%", `80%</font> (<font color="FFAABB">1000</font> Adena / 1 day(s))`, "%hp_period%", next, "%change_hp%", removeHP+hpGrade2,
		"%exp_recovery%", "none", "%exp_period%", "none", "%change_exp%", expGrade2,
		"%mp_recovery%", "none", "%mp_period%", "none", "%change_mp%", mpGrade2,
		"%objectId%", oid)
	if got := w.page(t, "manage recovery"); got != want {
		t.Fatalf("recovery page =\n%s\nwant\n%s", got, want)
	}

	// Mirror 2 is priced 3 days.
	want = fill(managerPage(t, "edit_other.htm"),
		"%tele%", `Stage 2</font> (<font color="FFAABB">6000</font> Adena / 3 day(s))`, "%tele_period%", next, "%change_tele%", removeTele+tele,
		"%support%", "none", "%support_period%", "none", "%change_support%", support2,
		"%item%", "none", "%item_period%", "none", "%change_item%", item,
		"%objectId%", oid)
	if got := w.page(t, "manage other"); got != want {
		t.Fatalf("other page =\n%s\nwant\n%s", got, want)
	}

	want = fill(managerPage(t, "deco.htm"),
		"%curtain%", "none", "%curtain_period%", "none", "%change_curtain%", curtains,
		"%fixture%", `Stage 1</font>&nbsp;(<font color="FFAABB">1300</font> Adena / 3 day(s))`, "%fixture_period%", next, "%change_fixture%", removeFix+fixtures,
		"%objectId%", oid)
	if got := w.page(t, "manage deco"); got != want {
		t.Fatalf("deco page =\n%s\nwant\n%s", got, want)
	}

	want = fill(managerPage(t, "functions-apply.htm"),
		"%name%", "Mirror (Teleportation Device)", "%cost%", "6000</font> Adena / 3 day(s)</font>)",
		"%use%", `Teleports clan members in a clan hall to the target <font color="00FFFF">Stage 2</font> staging area`,
		"%apply%", "other tele 2", "%objectId%", oid)
	if got := w.page(t, "manage other edit_tele 2"); got != want {
		t.Fatalf("offer page =\n%s\nwant\n%s", got, want)
	}
	if got := w.page(t, "manage deco fixtures_cancel"); got != fill(managerPage(t, "functions-cancel.htm"), "%apply%", "deco fixtures 0", "%objectId%", oid) {
		t.Fatalf("cancel offer =\n%s", got)
	}
	// The reference names an unchanged item, support or decoration level
	// after the command word.
	if got := w.page(t, "manage deco fixtures 1"); got != fill(managerPage(t, "functions-used.htm"), "%val%", "Stage fixtures", "%objectId%", oid) {
		t.Fatalf("unchanged fixtures page =\n%s", got)
	}
	if got := w.page(t, "manage other tele 2"); got != fill(managerPage(t, "functions-used.htm"), "%val%", "Stage 2", "%objectId%", oid) {
		t.Fatalf("unchanged teleport page =\n%s", got)
	}
}

// TestClanHallManagerServices: the functions overview shows the recovery
// levels; the teleport list holds the destinations of the rented level and
// takes the member there; item creation opens the level's buy list; a
// service the hall does not rent opens the disabled page.
func TestClanHallManagerServices(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(48 * time.Hour).UnixMilli()
	teleports, err := gamexml.LoadTeleports(datapack.Path(t, "data", "xml", "teleports.xml"))
	if err != nil {
		t.Fatal(err)
	}
	w := bootHallManager(t, hallManagerSetup{
		functions: [][5]int64{
			{1, 80, 1000, hallDayMs, end},
			{2, 15, 6500, hallDayMs, end},
			{5, 1, 7000, 7 * hallDayMs, end},
			{12, 2, 70000, hallDayMs, end},
		},
		extra: []gameservertest.Option{
			gameservertest.WithTeleports(teleports, nil, false, nil),
			gameservertest.WithBuyLists(buylist.List{ID: 235384, NPCID: hallManagerID}),
		},
	})
	oid := w.oid()

	want := fill(managerPage(t, "functions.htm"), "%npcId%", strconv.Itoa(hallManagerID), "%objectId%", oid,
		"%hp_regen%", "80", "%mp_regen%", "15", "%xp_regen%", "0")
	if got := w.page(t, "functions"); got != want {
		t.Fatalf("functions page =\n%s\nwant\n%s", got, want)
	}

	// The level 1 destinations of Black's list are its first five.
	want = "<html><body>&$556;<br><br>"
	for i, desc := range []string{"Village Square", "East Gate Entrance", "West Gate Entrance", "South Gate Entrance", "North Gate Entrance"} {
		want += `<a action="bypass -h npc_` + oid + `_teleport ` + strconv.Itoa(i) + `" msg="811;` + desc + `">` + desc + "</a><br1>"
	}
	want += "</body></html>"
	if got := w.page(t, "functions tele"); got != want {
		t.Fatalf("teleport list =\n%s\nwant\n%s", got, want)
	}
	frames := w.run(t, "functions item_creation "+strconv.Itoa(hallManagerID))
	frame, ok := firstOpcode(frames, serverpackets.OpcodeBuyList)
	if !ok {
		t.Fatalf("item creation answer % x has no BuyList", opcodes(frames))
	}
	if _, list, _, _ := decodeBuyList(t, frame); list != 235384 {
		t.Fatalf("buy list = %d, want 235384", list)
	}

	if got := w.page(t, "functions support"); got != fill(managerPage(t, "functions-disabled.htm"), "%objectId%", oid) {
		t.Fatalf("support page without the function =\n%s", got)
	}
	// Without support magic, support_back answers nothing of its own.
	if got := opcodes(w.run(t, "support_back")); !bytes.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("support_back answer = % x, want ActionFailed alone", got)
	}

	// Last, as it takes the member away from the manager.
	w.page(t, "functions tele")
	frames = w.bypass(t, npcCommand(w.manager, "teleport 2"))
	if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); !ok {
		t.Fatalf("teleport answer % x has no TeleportToLocation", opcodes(frames))
	}
}

// TestClanHallManagerSupportMagic: with support magic rented, the list
// shows the manager's MP; a listed skill is cast on the member on the
// manager's next AI tick, followed by the done page with the MP left. A
// command naming no number is told so.
func TestClanHallManagerSupportMagic(t *testing.T) {
	t.Parallel()
	defs, err := gamexml.LoadSkillDefinitions(datapack.Path(t, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().Add(48 * time.Hour).UnixMilli()
	w := bootHallManager(t, hallManagerSetup{
		functions: [][5]int64{{9, 1, 2500, hallDayMs, end}},
		extra:     []gameservertest.Option{gameservertest.WithAITask()},
	})
	tmpl := folkTemplate("ClanHallManagerNpc", hallManagerID)
	tmpl.Name, tmpl.MPMax = "Black", 1493
	w.manager = w.srv.SpawnCastingFolkNPCAt(t, tmpl, location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z}, defs)
	w.settleAI(t)
	drainUntilQuiet(t, w.c)
	oid := w.oid()
	mp := strconv.Itoa(int(w.manager.MPValue()))

	if got := w.page(t, "functions support"); got != fill(managerPage(t, "support1.htm"), "%mp%", mp, "%objectId%", oid) {
		t.Fatalf("support list =\n%s", got)
	}
	frames := w.bypass(t, npcCommand(w.manager, "support 4342 1"))
	if got := opcodes(frames); !bytes.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("support answer = % x, want ActionFailed alone until the AI acts", got)
	}
	frames = w.tickAI(t)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse); !ok {
		t.Fatalf("AI tick % x cast nothing", opcodes(frames))
	}
	// The page carries the MP the cast left the manager.
	done := pageIn(t, frames)
	const mpAt = `<font color="55FFFF">`
	start := strings.Index(done, mpAt)
	if start < 0 {
		t.Fatalf("done page =\n%s", done)
	}
	left := done[start+len(mpAt):]
	left = left[:strings.IndexByte(left, '<')]
	// Wind Walk takes its MP when it hits, after the page.
	if n, err := strconv.Atoi(left); err != nil || n > int(w.manager.MaxMPValue()) {
		t.Fatalf("MP left = %q, want at most the manager's %v", left, w.manager.MaxMPValue())
	}
	if done != fill(managerPage(t, "support-done.htm"), "%mp%", left, "%objectId%", oid) {
		t.Fatalf("done page =\n%s", done)
	}

	frames = w.run(t, "support x")
	if got := systemMessageTexts(t, frames); len(got) != 1 || got[0] != "Invalid skill, contact your server support." {
		t.Fatalf("malformed support notices = %q", got)
	}
}

// systemMessageTexts are the S1 texts of the system messages among frames.
func systemMessageTexts(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage || systemMessageID(f) != serverpackets.SystemMessageS1 {
			continue
		}
		out = append(out, systemMessageText(t, f))
	}
	return out
}
