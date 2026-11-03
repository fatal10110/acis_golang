package admin

import (
	"context"
	"database/sql"
	"encoding/binary"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: AdminClanHall.useAdminCommand / showClanHallSelectPage, with
// ClanHall.setOwner / free and Auction.endAuction. The shipped
// clanHalls.xml: Fortress of Resistance (21) is a siegable Dion hall;
// Moonstone (22), Onyx (23), Topaz (24) and Ruby (25) Halls are Gludio
// halls of size 30, grade 2, lease 500,000 and minimum bid 20,000,000.
// Moonstone's gates are gludio_castle_agit_001_001 and _002 and its one
// OWNER spawn is (-16029, 123642, -3101).

const (
	chKeepersID   = 0x70000031
	chRivalsID    = 0x70000032
	chBiddersID   = 0x70000033
	chGateA       = 24_220_001
	chGateB       = 24_220_002
	chUsage       = "Usage: //ch [set|remove|open|close|teleportto|end|siege chId]."
	chHallDesc    = "Clan hall located in the Town of Gludio"
	chWinningBid  = 25_000_000
	chBidderName  = "Bidman"
	chDayLayout   = "2006-01-02"
	chFortress    = 21
	chMoonstone   = 22
	chOnyx        = 23
	chTopaz       = 24
	chRuby        = 25
	chPaidAheadMs = int64(72 * time.Hour / time.Millisecond)
)

// chWorld is the //ch scenario: Admin, in clan inClan ("Keepers") or in
// none; Rivals owns Moonstone Hall, its lease paid until paidUntil;
// Bidders bid 25,000,000 on Onyx Hall; Moonstone, Onyx and Topaz have
// rows, their auctions ending at end.
type chWorld struct {
	srv       *gameservertest.Server
	gm        *testsupport.ScriptedClient
	objID     int32
	halls     *hallmodel.Table
	end       int64
	paidUntil int64
}

func bootClanHallAdmin(t *testing.T, inClan bool) *chWorld {
	t.Helper()
	return bootClanHallAdminWith(t, inClan, nil)
}

// bootClanHallAdminWith boots the //ch scenario with seed, more statements
// run after its rows, and extra, more boot options.
func bootClanHallAdminWith(t *testing.T, inClan bool, seed []string, extra ...gameservertest.Option) *chWorld {
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
	w := &chWorld{halls: halls, end: time.Now().Add(48 * time.Hour).UnixMilli()}
	w.paidUntil = time.Now().UnixMilli() + chPaidAheadMs
	srv, objID := bootAdmin(t, adminLevel, append([]gameservertest.Option{
		gameservertest.WithHTMLPages(shippedAdminPages(t, "clanhall.htm", "clanhalls.htm")),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithDoors(
			chGate(chGateA, "gludio_castle_agit_001_001", -60),
			chGate(chGateB, "gludio_castle_agit_001_002", 80),
		),
		gameservertest.WithClanSeed(func(db *sql.DB) { w.seed(t, db, inClan, seed) }),
	}, extra...)...)
	w.srv, w.objID, w.gm = srv, objID, srv.Client
	enterWorld(t, w.gm)
	return w
}

// chGate is an open door named after one of Moonstone Hall's gates,
// beside the spawn point so the GM sees its status.
func chGate(id int, name string, x int) *door.Template {
	return &door.Template{
		ID:       id,
		Name:     name,
		Kind:     door.KindDoor,
		Level:    1,
		Position: location.Location{X: x, Y: 100, Z: spawnZ},
		Coordinates: []location.Point{
			{X: x - 8, Y: 92}, {X: x + 8, Y: 92}, {X: x + 8, Y: 108}, {X: x - 8, Y: 108},
		},
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		Opened:   true,
		OpenKind: door.OpenNPC,
	}
}

func (w *chWorld) seed(t *testing.T, db *sql.DB, inClan bool, more []string) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed //ch: %s: %v", q, err)
		}
	}
	if inClan {
		exec("UPDATE characters SET clanid = ? WHERE char_name = 'Admin'", chKeepersID)
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
			SELECT ?, 'Keepers', 4, obj_Id FROM characters WHERE char_name = 'Admin'`, chKeepersID)
	}
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Rivals', 4)", chRivalsID)
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, auction_bid_at) VALUES (?, 'Bidders', 4, ?)", chBiddersID, chOnyx)
	exec("INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (?, ?, ?, 1, ?)", chMoonstone, chRivalsID, w.paidUntil, w.end)
	exec("INSERT INTO clanhall (id, endDate) VALUES (?, ?), (?, ?)", chOnyx, w.end, chTopaz, w.end)
	exec("INSERT INTO auctions (clanhall_id, bidder_name, clan_oid, clan_name, max_bid, time_bid) VALUES (?, ?, ?, 'Bidders', ?, 1)",
		chOnyx, chBidderName, chBiddersID, chWinningBid)
	for _, q := range more {
		exec(q)
	}
}

// ch sends //ch args as a clicked link and returns the frames it drew.
func (w *chWorld) ch(t *testing.T, args string) [][]byte {
	t.Helper()
	return exchange(t, w.gm, encodeBypass("admin_ch "+args))
}

// storedOwner is hall id's stored ownerId and endDate.
func (w *chWorld) storedOwner(t *testing.T, id int) (owner int32, endDate int64) {
	t.Helper()
	w.srv.FlushPersistence(t)
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT ownerId, endDate FROM clanhall WHERE id = ?", id).Scan(&owner, &endDate); err != nil {
		t.Fatal(err)
	}
	return owner, endDate
}

// chPage is the hall page as the reference fills it: each placeholder in
// turn, the auction rows matched without the closing %, which stays.
func chPage(t *testing.T, name string, id, owner string, paid bool, paidUntil int64, auction string) string {
	t.Helper()
	raw, ok := gameservertest.HTMLCache(t, shippedAdminPages(t, "clanhall.htm")).Get("data/html/admin/clanhall.htm")
	if !ok {
		t.Fatal("no admin/clanhall.htm in the HTML cache")
	}
	for _, r := range [][2]string{
		{"%name%", name},
		{"%id%", id},
		{"%size%", "30"},
		{"%grade%", "2"},
		{"%lease%", "500,000"},
		{"%loc%", "Gludio"},
		{"%desc%", chHallDesc},
		{"%defaultbid%", "20,000,000"},
		{"%owner%", owner},
		{"%paid%", strconv.FormatBool(paid)},
		{"%paiduntil%", time.UnixMilli(paidUntil).Format(chDayLayout)},
		{"%auction", auction},
	} {
		raw = strings.ReplaceAll(raw, r[0], r[1])
	}
	if !strings.Contains(raw, "</table>%") && !strings.Contains(raw, "</tr>%") {
		t.Fatalf("expected page keeps no %% after the auction rows:\n%s", raw)
	}
	return raw
}

// noBidRows are the auction rows of an auction without a bid.
func noBidRows(end int64) string {
	return "<tr><td width=150>Bid: 0</td><td width=120>Bidders: 0</td></tr><tr><td>End date: " +
		time.UnixMilli(end).Format(chDayLayout) + "</td><td>Winner: none</td></tr>"
}

// chLastPage requires frames to end with an admin page and returns it.
func chLastPage(t *testing.T, frames [][]byte) string {
	t.Helper()
	if len(frames) == 0 {
		t.Fatal("no frames, want the hall page last")
	}
	return htmlBody(t, frames[len(frames)-1])
}

// wantTextThenPage requires frames to be the plain-text message text, then
// a page, and returns the page.
func wantTextThenPage(t *testing.T, frames [][]byte, text string) string {
	t.Helper()
	if len(frames) != 2 {
		t.Fatalf("frames = %x, want the message %q then the page", testsupport.FrameOpcodes(frames), text)
	}
	assertTexts(t, frames[:1], text)
	return htmlBody(t, frames[1])
}

// doorStates returns the door ids the DoorStatusUpdate frames among
// frames report, with their closed flag.
func doorStates(frames [][]byte) map[int32]bool {
	states := map[int32]bool{}
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeDoorStatusUpdate || len(f) < 21 {
			continue
		}
		states[int32(binary.LittleEndian.Uint32(f[17:]))] = binary.LittleEndian.Uint32(f[5:]) == 1
	}
	return states
}

func hasOpcode(frames [][]byte, op byte) bool {
	return slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == op })
}

// TestAdminClanHallPagesAndCommands drives //ch through each action
// against the shipped pages and halls: the list, a hall's page with and
// without a bid, set and its refusals, remove, end, the gates and the
// teleport.
func TestAdminClanHallPagesAndCommands(t *testing.T) {
	t.Parallel()
	w := bootClanHallAdmin(t, true)

	// The list: each town's halls by id, an owned one marked *, a hall
	// holding an auction with its bid count, end date and raw minimum bid.
	raw, ok := gameservertest.HTMLCache(t, shippedAdminPages(t, "clanhalls.htm")).Get("data/html/admin/clanhalls.htm")
	if !ok {
		t.Fatal("no admin/clanhalls.htm in the HTML cache")
	}
	all := slices.Clone(w.halls.All())
	slices.SortFunc(all, func(a, b *hallmodel.Hall) int { return a.ID - b.ID })
	bids := map[int]int{chMoonstone: 0, chOnyx: 1, chTopaz: 0}
	var list strings.Builder
	for _, town := range []string{"Aden", "Dion", "Giran", "Gludin", "Gludio", "Goddard", "Oren", "Rune", "Schuttgart"} {
		list.WriteString(`<font color="LEVEL">` + town + `</font><br><table width=270><tr><td width=130>Clan Hall Name</td><td width=70>End Date</td><td width=70>Min Bid</td></tr>`)
		for _, h := range all {
			if !strings.EqualFold(h.Town, town) {
				continue
			}
			mark := ""
			if h.ID == chMoonstone {
				mark = "*"
			}
			list.WriteString(`<tr><td><a action="bypass -h admin_ch ` + strconv.Itoa(h.ID) + `">` + h.Name + mark)
			if n, ok := bids[h.ID]; ok {
				list.WriteString(" [" + strconv.Itoa(n) + "]</a></td><td>" + time.UnixMilli(w.end).Format(chDayLayout) + "</td><td>20000000</td></tr>")
			} else {
				list.WriteString("</a></td><td>-</td><td>-</td></tr>")
			}
		}
		list.WriteString("</table>")
	}
	wantList := strings.ReplaceAll(raw, "%AGIT_LIST%", list.String())
	if !strings.Contains(wantList, "Topaz Hall [0]") || !strings.Contains(wantList, "Fortress of Resistance</a>") {
		t.Fatalf("expected list misses its rows:\n%s", wantList)
	}
	for _, payload := range [][]byte{encodeBuildCmd("ch"), encodeBypass("admin_ch"), encodeBypass("admin_ch 99")} {
		frames := exchange(t, w.gm, payload)
		if len(frames) != 1 {
			t.Fatalf("list frames = %x, want one page", testsupport.FrameOpcodes(frames))
		}
		if got := htmlBody(t, frames[0]); got != wantList {
			t.Fatalf("list =\n%s\nwant\n%s", got, wantList)
		}
	}

	// Onyx Hall, free, with Bidders' bid standing highest.
	onyxBid := "<tr><td width=150>Bid: 25,000,000</td><td width=120>Bidders: 1</td></tr><tr><td>End date: " +
		time.UnixMilli(w.end).Format(chDayLayout) + "</td><td>Winner: " + chBidderName + "</td></tr>"
	frames := w.ch(t, "23")
	if len(frames) != 1 {
		t.Fatalf("Onyx frames = %x, want one page", testsupport.FrameOpcodes(frames))
	}
	if got, want := htmlBody(t, frames[0]), chPage(t, "Onyx Hall", "23", "None", false, 0, onyxBid); got != want {
		t.Fatalf("Onyx page =\n%s\nwant\n%s", got, want)
	}
	// Moonstone Hall, Rivals', its auction without a bid.
	moonstonePage := chPage(t, "Moonstone Hall", "22", "Rivals", true, w.paidUntil, noBidRows(w.end))
	if got := htmlBody(t, w.ch(t, "22")[0]); got != moonstonePage {
		t.Fatalf("Moonstone page =\n%s\nwant\n%s", got, moonstonePage)
	}

	// set: no selection, then the GM selecting itself, a member of Keepers.
	frames = w.ch(t, "set 24")
	if len(frames) != 2 {
		t.Fatalf("set without a target = %x, want 144 then the page", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageTargetIncorrect)
	exchange(t, w.gm, encodeAction(w.objID))
	if got := wantTextThenPage(t, w.ch(t, "set 22"), "This ClanHall isn't free."); got != moonstonePage {
		t.Fatalf("page after the refused set =\n%s", got)
	}
	frames = w.ch(t, "set 24")
	if !hasOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate) {
		t.Fatalf("set frames = %x, want the clan header refreshed", testsupport.FrameOpcodes(frames))
	}
	topazPaid, _ := w.srv.Halls.View(chTopaz)
	if got, want := chLastPage(t, frames), chPage(t, "Topaz Hall", "24", "Keepers", true, topazPaid.PaidUntil, noBidRows(0)); got != want {
		t.Fatalf("Topaz page after set =\n%s\nwant\n%s", got, want)
	}
	if owner, _ := w.storedOwner(t, chTopaz); owner != chKeepersID {
		t.Fatalf("Topaz stored owner = %d, want Keepers", owner)
	}
	wantTextThenPage(t, w.ch(t, "set 25"), "Your target already owns a ClanHall.")

	// remove: Topaz goes free and its auction starts over, a week from now;
	// again, it is already free.
	frames = w.ch(t, "remove 24")
	if !hasOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate) {
		t.Fatalf("remove frames = %x, want the clan header refreshed", testsupport.FrameOpcodes(frames))
	}
	if got := chLastPage(t, frames); !strings.Contains(got, "Owner: None") || !strings.Contains(got, "Paid: false") {
		t.Fatalf("Topaz page after remove =\n%s", got)
	}
	freed, _ := w.srv.Halls.View(chTopaz)
	if owner, end := w.storedOwner(t, chTopaz); owner != 0 || end != freed.EndDate || end <= w.end {
		t.Fatalf("Topaz stored after remove = owner %d end %d, want free, auction restarted to end %d", owner, end, freed.EndDate)
	}
	wantTextThenPage(t, w.ch(t, "remove 24"), "This ClanHall is already free.")

	// end: the siegable hall holds no auction; Onyx goes to its bidder.
	if got := wantTextThenPage(t, w.ch(t, "end 21"), "This ClanHall doesn't hold an auction."); !strings.Contains(got, "This clanhall doesn't have Auction.</td></tr>%") {
		t.Fatalf("Fortress page =\n%s", got)
	}
	if got := chLastPage(t, w.ch(t, "end 23")); !strings.Contains(got, "Owner: Bidders") || !strings.Contains(got, "Winner: none") {
		t.Fatalf("Onyx page after end =\n%s", got)
	}
	if owner, _ := w.storedOwner(t, chOnyx); owner != chBiddersID {
		t.Fatalf("Onyx stored owner = %d, want Bidders", owner)
	}

	// The gates close, then open, each with the page last.
	frames = w.ch(t, "close 22")
	if got := doorStates(frames); len(got) != 2 || !got[chGateA] || !got[chGateB] {
		t.Fatalf("close 22 doors = %v (frames %x), want both gates closed", got, testsupport.FrameOpcodes(frames))
	}
	if chLastPage(t, frames) != moonstonePage {
		t.Fatal("close 22 did not end on Moonstone's page")
	}
	frames = w.ch(t, "open 22")
	if got := doorStates(frames); len(got) != 2 || got[chGateA] || got[chGateB] {
		t.Fatalf("open 22 doors = %v (frames %x), want both gates open", got, testsupport.FrameOpcodes(frames))
	}

	// teleportto: to Moonstone's owner restart point, the page after.
	frames = w.ch(t, "teleportto 22")
	if at := teleportOf(t, frames, w.objID); at != [3]int32{-16029, 123642, -3101} {
		t.Fatalf("teleport to %v, want Moonstone's owner point", at)
	}
	if chLastPage(t, frames) != moonstonePage {
		t.Fatal("teleportto 22 did not end on Moonstone's page")
	}
}

// TestAdminClanHallRefusals: an id that does not parse gets the usage
// alone; siege checks its action and that the hall is siegable; set on a
// selected player outside any clan is an incorrect target.
func TestAdminClanHallRefusals(t *testing.T) {
	t.Parallel()
	w := bootClanHallAdmin(t, false)

	assertTexts(t, w.ch(t, "x"), chUsage)
	assertTexts(t, w.ch(t, "set x"), chUsage)
	wantTextThenPage(t, w.ch(t, "siege 22"), "Usage: //ch siege start|end chId.")
	wantTextThenPage(t, w.ch(t, "siege 5 22"), "Usage: //ch siege start|end chId.")
	wantTextThenPage(t, w.ch(t, "siege start 22"), "This ClanHall isn't siegable.")
	// A siegable hall no script runs the siege of: start has no siege to
	// start and gets the usage alone, end finds none being fought.
	assertTexts(t, w.ch(t, "siege start 21"), chUsage)
	if page := wantTextThenPage(t, w.ch(t, "siege end 21"), "This ClanHall isn't currently besieged."); !strings.Contains(page, "Fortress of Resistance") {
		t.Fatalf("siege end 21 page =\n%s", page)
	}
	// An unknown action shows the page.
	if frames := w.ch(t, "bogus 22"); len(frames) != 1 || !strings.Contains(htmlBody(t, frames[0]), "Moonstone Hall") {
		t.Fatalf("bogus 22 frames = %x, want the hall page", testsupport.FrameOpcodes(frames))
	}

	exchange(t, w.gm, encodeAction(w.objID))
	frames := w.ch(t, "set 24")
	if len(frames) != 2 {
		t.Fatalf("set on a clanless target = %x, want 144 then the page", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageTargetIncorrect)
	if owner, _ := w.storedOwner(t, chTopaz); owner != 0 {
		t.Fatalf("Topaz stored owner = %d, want none", owner)
	}
}
