package npcs

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: Auctioneer.onBypassFeedback / showChatWindow /
// showAuctionsList / showSelectedItems, Auction (setBid, cancelBid,
// endAuction, removeBids, confirmAuction, cancelAuction), ClanHall
// (setOwner, free, payFee) and ClanHallManager (getAuctionableClanHalls).
// Moonstone Hall (22) and Onyx Hall (23) are Gludio halls: minimum bid
// 20,000,000, lease 500,000, size 30, "Clan hall located in the Town of
// Gludio". A cancelled or lost bid comes back as (int)(bid * 0.9).

const (
	auctioneerNpcID = 30767
	auctionClanID   = 0x70000021
	rivalClanID     = 0x70000022
	auctionAdenaObj = 0x70000023
	rivalAdenaObj   = 0x70000024
	moonstone       = 22
	onyx            = 23
	hourMs          = int64(time.Hour / time.Millisecond)
	dayMs           = 24 * hourMs
)

// hallRow is one seeded clanhall row.
type hallRow struct {
	id, owner             int
	paidUntil             int64
	paid                  bool
	sellerBid             int
	sellerName, sellerCln string
	endDate               int64
}

// bidRow is one seeded auctions row.
type bidRow struct {
	hall, clan     int
	name, clanName string
	bid            int
	time           int64
}

// auctionSetup is the world an auction scenario boots with: Talker leads
// "Bidders" (clanLevel; 0 for no clan), whose warehouse holds adena, and
// "Rivals" exists beside it with rivalAdena.
type auctionSetup struct {
	clanLevel  int
	adena      int
	rivalAdena int
	// bidAt and rivalBidAt are the clans' auction_bid_at.
	bidAt, rivalBidAt int
	halls             []hallRow
	bids              []bidRow
}

type auctionWorld struct {
	*folkWorld
	auctioneer *npc.Folk
}

// auctionPages are the shipped auction pages.
func auctionPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "auction")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		pages["auction/"+e.Name()] = string(data)
	}
	return pages
}

func auctionPage(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "auction", name))
	if err != nil {
		t.Fatal(err)
	}
	// The page cache reads a page with its line ends as "\n" and one at
	// the end.
	page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return page
}

func bootAuction(t *testing.T, s auctionSetup, extra ...gameservertest.Option) *auctionWorld {
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
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedAuction(t, db, s) }),
		noBypassReuse,
	}, extra...)
	w := bootFolkWorld(t, auctionPages(t), opts...)
	tmpl := folkTemplate("Auctioneer", auctioneerNpcID)
	tmpl.Name = "Auctioneer"
	return &auctionWorld{folkWorld: w, auctioneer: w.spawnFolk(t, tmpl, 60)}
}

func seedAuction(t *testing.T, db *sql.DB, s auctionSetup) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed auction: %s: %v", q, err)
		}
	}
	if s.clanLevel > 0 {
		exec("UPDATE characters SET clanid = ? WHERE char_name = 'Talker'", auctionClanID)
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, auction_bid_at)
			SELECT ?, 'Bidders', ?, obj_Id, ? FROM characters WHERE char_name = 'Talker'`, auctionClanID, s.clanLevel, s.bidAt)
	}
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, auction_bid_at) VALUES (?, 'Rivals', 3, ?)", rivalClanID, s.rivalBidAt)
	if s.adena > 0 {
		exec("INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES (?, ?, ?, ?, 'CLANWH', 0)",
			auctionClanID, auctionAdenaObj, item.AdenaID, s.adena)
	}
	if s.rivalAdena > 0 {
		exec("INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES (?, ?, ?, ?, 'CLANWH', 0)",
			rivalClanID, rivalAdenaObj, item.AdenaID, s.rivalAdena)
	}
	for _, h := range s.halls {
		exec(`INSERT INTO clanhall (id, ownerId, paidUntil, paid, sellerBid, sellerName, sellerClanName, endDate)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, h.id, h.owner, h.paidUntil, h.paid, h.sellerBid, h.sellerName, h.sellerCln, h.endDate)
	}
	for _, b := range s.bids {
		exec("INSERT INTO auctions (clanhall_id, bidder_name, clan_oid, clan_name, max_bid, time_bid) VALUES (?, ?, ?, ?, ?, ?)",
			b.hall, b.name, b.clan, b.clanName, b.bid, b.time)
	}
}

// everyHall seeds a free row for each clan hall id, each auction ending at
// end, but for the rows in over.
func everyHall(end int64, over ...hallRow) []hallRow {
	var rows []hallRow
	for id := 21; id <= 64; id++ {
		row := hallRow{id: id, endDate: end}
		for _, o := range over {
			if o.id == id {
				row = o
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// open talks to the auctioneer and returns its chat window.
func (w *auctionWorld) open(t *testing.T) string {
	t.Helper()
	w.selectFolk(t, w.auctioneer)
	return pageIn(t, w.talk(t, w.auctioneer, false))
}

// bypass sends the auctioneer command and returns the answer.
func (w *auctionWorld) bypass(t *testing.T, command string) [][]byte {
	t.Helper()
	w.c.Send(encodeBypass("npc_" + w.oid() + "_" + command))
	return drainFrames(t, w.c)
}

// page sends the auctioneer command and returns the page it opens.
func (w *auctionWorld) page(t *testing.T, command string) string {
	t.Helper()
	return pageIn(t, w.bypass(t, command))
}

func (w *auctionWorld) oid() string { return strconv.Itoa(int(w.auctioneer.ObjectID())) }

func pageIn(t *testing.T, frames [][]byte) string {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("no NpcHtmlMessage among opcodes % x", opcodes(frames))
	}
	_, html, _ := htmlMessage(t, frame)
	return html
}

// messages returns the system message ids among frames, in order.
func messages(frames [][]byte) []int32 {
	var ids []int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, systemMessageID(f))
		}
	}
	return ids
}

func wantMessages(t *testing.T, what string, frames [][]byte, ids ...int32) {
	t.Helper()
	got := messages(frames)
	if len(got) != len(ids) {
		t.Fatalf("%s: system messages %v, want %v (opcodes % x)", what, got, ids, opcodes(frames))
	}
	for i := range ids {
		if got[i] != ids[i] {
			t.Fatalf("%s: system messages %v, want %v", what, got, ids)
		}
	}
}

func fill(page string, pairs ...string) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		page = strings.ReplaceAll(page, pairs[i], pairs[i+1])
	}
	return page
}

func clanAdena(t *testing.T, w *auctionWorld, clanID int) int64 {
	t.Helper()
	w.srv.FlushItems(t)
	var n int64
	err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = ? AND loc = 'CLANWH'", clanID, item.AdenaID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func storedHall(t *testing.T, w *auctionWorld, id int) hallRow {
	t.Helper()
	w.srv.FlushPersistence(t)
	var r hallRow
	err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT id, ownerId, paidUntil, paid, sellerBid, sellerName, sellerClanName, endDate FROM clanhall WHERE id = ?", id).
		Scan(&r.id, &r.owner, &r.paidUntil, &r.paid, &r.sellerBid, &r.sellerName, &r.sellerCln, &r.endDate)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func storedBids(t *testing.T, w *auctionWorld, hall int) []bidRow {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.QueryContext(context.Background(),
		"SELECT clanhall_id, clan_oid, bidder_name, clan_name, max_bid, time_bid FROM auctions WHERE clanhall_id = ? ORDER BY clan_oid", hall)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []bidRow
	for rows.Next() {
		var b bidRow
		if err := rows.Scan(&b.hall, &b.clan, &b.name, &b.clanName, &b.bid, &b.time); err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func storedBidAt(t *testing.T, w *auctionWorld, clanID int) int {
	t.Helper()
	w.srv.FlushPersistence(t)
	var at int
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT auction_bid_at FROM clan_data WHERE clan_id = ?", clanID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

// pledgeHall is the clan hall id a PledgeShowInfoUpdate frame carries.
func pledgeHall(frame []byte) int32 {
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // clan
	r.ReadInt32() // crest
	r.ReadInt32() // level
	r.ReadInt32() // castle
	return r.ReadInt32()
}

func endDay(ms int64) string  { return time.UnixMilli(ms).Format("2006-01-02") }
func endTime(ms int64) string { return time.UnixMilli(ms).Format("02-01-2006 15:04") }

// TestAuctioneerChatAndList: the chat window names the auctioneer; the
// list shows the free halls holding an auction, by id, 15 a page with a
// link to each page, an owned hall not for sale left out. Each row is the
// town, the hall linking to its details with its bid count, the end day
// and the minimum bid.
func TestAuctioneerChatAndList(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2,
		halls:     everyHall(end, hallRow{id: onyx, owner: rivalClanID, paid: true, paidUntil: end}),
		bids: []bidRow{
			{moonstone, rivalClanID, "Rival", "Rivals", 21_000_000, 1},
			{moonstone, 0x70000099, "Other", "Others", 22_000_000, 2},
		},
	})
	oid := w.oid()
	chat := w.open(t)
	if want := fill(auctionPage(t, "auction.htm"), "%objectId%", oid, "%npcId%", strconv.Itoa(auctioneerNpcID), "%npcname%", "Auctioneer"); chat != want {
		t.Fatalf("chat window =\n%s\nwant\n%s", chat, want)
	}

	list := w.page(t, "list")
	first := `<tr><td><font color="aaaaff">Gludio</font></td><td><font color="ffffaa"><a action="bypass -h npc_` + oid +
		`_bidding 22">Moonstone Hall [2]</a></font></td><td>` + endDay(end) + `</td><td><font color="aaffff">20000000</font></td></tr>`
	if !strings.Contains(list, `<table width=280>`+first) {
		t.Fatalf("list page does not open with Moonstone Hall's row %q:\n%s", first, list)
	}
	if strings.Contains(list, "_bidding 23\"") || strings.Contains(list, "_bidding 21\"") {
		t.Fatalf("list page shows an owned or siegable hall:\n%s", list)
	}
	if got := strings.Count(list, "<tr><td><font color=\"aaaaff\">"); got != 15 {
		t.Fatalf("list page rows = %d, want 15", got)
	}
	pages := `</table><table width=280><tr>`
	for j := 1; j <= 3; j++ {
		pages += `<td align=center><a action="bypass -h npc_` + oid + `_list ` + strconv.Itoa(j) + `"> Page ` + strconv.Itoa(j) + ` </a></td>`
	}
	if !strings.Contains(list, pages+"</tr></table>") {
		t.Fatalf("list page lacks the three page links:\n%s", list)
	}
	if !strings.Contains(list, `action="bypass -h npc_`+oid+`_start"`) {
		t.Fatalf("list page lacks the back link:\n%s", list)
	}
	// 37 halls: the third page holds the last 7.
	last := w.page(t, "list 3")
	if got := strings.Count(last, "<tr><td><font color=\"aaaaff\">"); got != 7 {
		t.Fatalf("third list page rows = %d, want 7", got)
	}
}

// TestAuctioneerWithNothingForSale: with no hall up for auction every
// command is refused with a notice.
func TestAuctioneerWithNothingForSale(t *testing.T) {
	t.Parallel()
	w := bootAuction(t, auctionSetup{clanLevel: 2})
	w.open(t)
	frames := w.bypass(t, "list")
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatal("list opened with nothing for sale")
	}
	wantMessages(t, "list", frames, serverpackets.SystemMessageNoClanHallsUpForAuction)
}

// TestAuctionBidRebidAndCancel: a clan leader bids through the details and
// the form: a bid not above the minimum, or not above the clan's own, is
// refused; a placed bid takes what it adds from the clan warehouse and is
// stored under the bidder's name. Cancelling returns the bid minus 10%.
func TestAuctionBidRebidAndCancel(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{clanLevel: 2, adena: 30_000_000, halls: []hallRow{{id: moonstone, endDate: end}}})
	oid := w.oid()
	w.open(t)
	w.page(t, "list")
	info := w.page(t, "bidding 22")
	if strings.Contains(info, "%AGIT") || strings.Contains(info, "%OWNER") {
		t.Fatalf("details page left a placeholder:\n%s", info)
	}
	for _, want := range []string{
		">Moonstone Hall<", ">30 rating<", ">500000<", "<td>Gludio</td>", ">" + endTime(end) + "<", ">20000000<",
		"bypass -h npc_" + oid + "_bidlist 22", "bypass -h npc_" + oid + "_bid1 22", "bypass -h npc_" + oid + "_list",
	} {
		if !strings.Contains(info, want) {
			t.Fatalf("details page lacks %q:\n%s", want, info)
		}
	}
	form := w.page(t, "bid1 22")
	want := fill(auctionPage(t, "AgitBid1.htm"),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_bidding 22",
		"%PLEDGE_ADENA%", "30000000",
		"%AGIT_AUCTION_MINBID%", "20000000",
		"npc_%objectId%_bid", "npc_"+oid+"_bid 22")
	if form != want {
		t.Fatalf("bid form =\n%s\nwant\n%s", form, want)
	}

	wantMessages(t, "bid at the minimum", w.bypass(t, "bid 22 20000000"), serverpackets.SystemMessageBidPriceMustBeHigher)
	wantMessages(t, "bid", w.bypass(t, "bid 22 25000000"), serverpackets.SystemMessageBidInClanHallAuction)
	if got := clanAdena(t, w, auctionClanID); got != 5_000_000 {
		t.Fatalf("clan adena after the bid = %d, want 5000000", got)
	}
	wantMessages(t, "same bid again", w.bypass(t, "bid 22 25000000"), serverpackets.SystemMessageBidPriceMustBeHigher)
	wantMessages(t, "rebid", w.bypass(t, "bid 22 28000000"), serverpackets.SystemMessageBidInClanHallAuction)
	if got := clanAdena(t, w, auctionClanID); got != 2_000_000 {
		t.Fatalf("clan adena after the rebid = %d, want 2000000", got)
	}
	bids := storedBids(t, w, moonstone)
	if len(bids) != 1 || bids[0].clan != auctionClanID || bids[0].name != "Talker" || bids[0].clanName != "Bidders" || bids[0].bid != 28_000_000 {
		t.Fatalf("stored bids = %+v, want Talker of Bidders at 28000000", bids)
	}
	if at := storedBidAt(t, w, auctionClanID); at != moonstone {
		t.Fatalf("clan auction_bid_at = %d, want %d", at, moonstone)
	}
	wantMessages(t, "more than the warehouse holds", w.bypass(t, "bid 22 31000000"), serverpackets.SystemMessageNotEnoughAdenaInClanWarehouse)

	// Back to the clan's own page, then cancel.
	w.page(t, "bidding 22")
	w.page(t, "list")
	w.page(t, "start")
	mine := w.page(t, "selectedItems")
	for _, want := range []string{">Moonstone Hall<", ">28000000<", ">20000000<", "bypass -h npc_" + oid + "_cancelBid", "bypass -h npc_" + oid + "_rebid"} {
		if !strings.Contains(mine, want) {
			t.Fatalf("bid info page lacks %q:\n%s", want, mine)
		}
	}
	cancel := w.page(t, "cancelBid")
	if want := fill(auctionPage(t, "AgitBidCancel.htm"), "%AGIT_BID%", "28000000", "%AGIT_BID_REMAIN%", "25200000",
		"%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_selectedItems", "%objectId%", oid); cancel != want {
		t.Fatalf("cancel page =\n%s\nwant\n%s", cancel, want)
	}
	wantMessages(t, "cancel", w.bypass(t, "doCancelBid"), serverpackets.SystemMessageCanceledBid)
	if got := clanAdena(t, w, auctionClanID); got != 27_200_000 {
		t.Fatalf("clan adena after the cancel = %d, want 27200000", got)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids after the cancel = %+v, want none", bids)
	}
	if at := storedBidAt(t, w, auctionClanID); at != 0 {
		t.Fatalf("clan auction_bid_at after the cancel = %d, want 0", at)
	}
}

// TestAuctionBidRefusals: the bid form is refused, with the list and a
// notice, to a clan below level 2, to one that bid on another hall and to
// one owning a hall; a member without the auction right is refused every
// clan command. A hall owned and not for sale opens the form but takes no
// bid.
func TestAuctionBidRefusals(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	halls := everyHall(end, hallRow{id: onyx, owner: rivalClanID, paid: true, paidUntil: end})
	for _, tt := range []struct {
		name  string
		setup auctionSetup
		cmd   string
		want  int32
	}{
		{"clan level 1", auctionSetup{clanLevel: 1, halls: halls}, "bid1 22", serverpackets.SystemMessageAuctionOnlyClanLevel2Higher},
		{"bid elsewhere", auctionSetup{
			clanLevel: 2, bidAt: 24, halls: halls,
			bids: []bidRow{{24, auctionClanID, "Talker", "Bidders", 21_000_000, 1}},
		}, "bid1 22", serverpackets.SystemMessageAlreadySubmittedBid},
		{
			"owns a hall",
			auctionSetup{clanLevel: 2, halls: everyHall(end, hallRow{id: 24, owner: auctionClanID, paid: true, paidUntil: end})},
			"bid1 22", serverpackets.SystemMessageCannotParticipateInAuction,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootAuction(t, tt.setup)
			w.open(t)
			w.page(t, "list")
			w.page(t, "bidding 22")
			frames := w.bypass(t, tt.cmd)
			if page := pageIn(t, frames); !strings.Contains(page, "_bidding 22\">Moonstone Hall") {
				t.Fatalf("refusal page is not the list:\n%s", page)
			}
			wantMessages(t, tt.cmd, frames, tt.want)
		})
	}

	t.Run("no clan", func(t *testing.T) {
		t.Parallel()
		w := bootAuction(t, auctionSetup{halls: halls})
		w.open(t)
		frames := w.bypass(t, "selectedItems")
		pageIn(t, frames)
		wantMessages(t, "selectedItems", frames, serverpackets.SystemMessageCannotParticipateInAuction)
	})

	// The form was opened while the hall was free; it changed hands since.
	t.Run("hall not for sale", func(t *testing.T) {
		t.Parallel()
		w := bootAuction(t, auctionSetup{clanLevel: 2, adena: 30_000_000, halls: everyHall(end)})
		w.open(t)
		w.page(t, "list")
		w.page(t, "bidding 23")
		w.page(t, "bid1 23")
		rival, ok := w.srv.Clans.Table().Get(rivalClanID)
		if !ok || !w.srv.Halls.SetOwner(onyx, rival) {
			t.Fatal("give Onyx Hall to Rivals")
		}
		wantMessages(t, "bid", w.bypass(t, "bid 23 25000000"), serverpackets.SystemMessageCannotParticipateInAuction)
		if got := clanAdena(t, w, auctionClanID); got != 30_000_000 {
			t.Fatalf("clan adena = %d, want 30000000 untouched", got)
		}
		if bids := storedBids(t, w, onyx); len(bids) != 0 {
			t.Fatalf("stored bids = %+v, want none", bids)
		}
	})
}

// TestAuctionBidderListOrder: the bidders list reads the bids in the
// reference bid map's order: by clan id bucket (id ^ id>>>16, low 4 bits),
// then by when each bid was first made, here the stored order, highest
// bid first.
func TestAuctionBidderListOrder(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	t1 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local).UnixMilli()
	t2 := time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local).UnixMilli()
	t3 := time.Date(2026, 10, 3, 12, 0, 0, 0, time.Local).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, halls: []hallRow{{id: moonstone, endDate: end}},
		bids: []bidRow{
			{moonstone, 0x70000031, "Ann", "Bucket1", 30_000_000, t1}, // bucket 1
			{moonstone, 0x70000025, "Bob", "Bucket5", 25_000_000, t2}, // bucket 5
			{moonstone, 0x70000020, "Cid", "Bucket0", 21_000_000, t3}, // bucket 0
		},
	})
	oid := w.oid()
	w.open(t)
	w.page(t, "list")
	w.page(t, "bidding 22")
	list := w.page(t, "bidlist 22")
	row := func(clan, name string, at int64) string {
		return "<tr><td width=90 align=center>" + clan + "</td><td width=90 align=center>" + name +
			"</td><td width=90 align=center>" + endDay(at) + "</td></tr>"
	}
	want := fill(auctionPage(t, "AgitBidderList.htm"),
		"%AGIT_LIST%", row("Bucket0", "Cid", t3)+row("Bucket1", "Ann", t1)+row("Bucket5", "Bob", t2),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_bidding 22",
		"%objectId%", oid)
	if list != want {
		t.Fatalf("bidders list =\n%s\nwant\n%s", list, want)
	}
}

// TestAuctionSaleRegistersOnDeposit: the owner sets up a sale through its
// own page, the deposit page and the price form; the hall is put up for
// auction only once the confirmation takes the deposit, the hall's lease.
func TestAuctionSaleRegistersOnDeposit(t *testing.T) {
	t.Parallel()
	paid := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, adena: 1_000_000,
		halls: everyHall(paid, hallRow{id: moonstone, owner: auctionClanID, paid: true, paidUntil: paid, endDate: paid}),
	})
	oid := w.oid()
	w.open(t)
	info := w.page(t, "selectedItems")
	if want := fill(auctionPage(t, "AgitInfo.htm"), "%AGIT_NAME%", "Moonstone Hall", "%AGIT_OWNER_PLEDGE_NAME%", "Bidders",
		"%OWNER_PLEDGE_MASTER%", "Talker", "%AGIT_SIZE%", "30", "%AGIT_LEASE%", "500000", "%AGIT_LOCATION%", "Gludio",
		"%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_start", "%objectId%", oid); info != want {
		t.Fatalf("hall info page =\n%s\nwant\n%s", info, want)
	}
	if sale := w.page(t, "sale"); !strings.Contains(sale, ">500000</font> adena and there are currently <font color=\"aaccff\">1000000<") {
		t.Fatalf("deposit page:\n%s", sale)
	}
	if price := w.page(t, "sale2"); !strings.Contains(price, ">500000</font> adena and the last trading price was <font color=\"aaccff\">500000<") {
		t.Fatalf("price page:\n%s", price)
	}
	now := w.srv.Halls.Now()
	summary := w.page(t, "auction 7 30000000")
	for _, want := range []string{
		">30000000</font> adena", ">20000000</font> adena", "7 days", endTime(now + 7*dayMs),
		"bypass -h npc_" + oid + "_sale2", "bypass -h npc_" + oid + "_confirmAuction",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("sale summary lacks %q:\n%s", want, summary)
		}
	}
	if v, _ := w.srv.Halls.View(moonstone); v.Seller != nil {
		t.Fatal("hall put up for sale before the deposit")
	}

	frames := w.bypass(t, "confirmAuction")
	saleInfo := pageIn(t, frames)
	for _, want := range []string{">Moonstone Hall<", ">Bidders<", ">Talker<", ">30000000<", "bypass -h npc_" + oid + "_bidlist 22", "bypass -h npc_" + oid + "_cancelAuction"} {
		if !strings.Contains(saleInfo, want) {
			t.Fatalf("sale info page lacks %q:\n%s", want, saleInfo)
		}
	}
	wantMessages(t, "confirm", frames, serverpackets.SystemMessageRegisteredForClanHall)
	if got := clanAdena(t, w, auctionClanID); got != 500_000 {
		t.Fatalf("clan adena after the deposit = %d, want 500000", got)
	}
	row := storedHall(t, w, moonstone)
	if row.owner != auctionClanID || row.sellerBid != 30_000_000 || row.sellerName != "Talker" || row.sellerCln != "Bidders" || row.endDate != now+7*dayMs {
		t.Fatalf("stored hall = %+v, want the sale at 30000000 ending %d", row, now+7*dayMs)
	}
	v, _ := w.srv.Halls.View(moonstone)
	if v.MinBid != 30_000_000 {
		t.Fatalf("minimum bid = %d, want the seller's 30000000", v.MinBid)
	}
}

// TestAuctionEndAwardsTheHall: at the end the highest bidder gets the hall
// with a week's lease paid; every other bidder gets its bid back minus 10%
// and every bidding clan is told who won. The lease then falls due weekly:
// unpaid, a day's grace with a notice, then the hall is lost and goes back
// up for auction.
func TestAuctionEndAwardsTheHall(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, bidAt: moonstone, rivalBidAt: moonstone,
		halls: []hallRow{{id: moonstone, endDate: end}},
		bids: []bidRow{
			{moonstone, auctionClanID, "Talker", "Bidders", 25_000_000, 1},
			{moonstone, rivalClanID, "Rival", "Rivals", 22_000_000, 2},
		},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	w.srv.Advance(t, time.Hour+time.Minute)
	frames := drainFrames(t, w.c)
	wantMessages(t, "award", frames, serverpackets.SystemMessageClanHallAwardedToClanS1)
	var order []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage || f[0] == serverpackets.OpcodePledgeShowInfoUpdate {
			order = append(order, f[0])
		}
	}
	if string(order) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate}) {
		t.Fatalf("award frames % x, want the notice then the clan header", order)
	}
	award, _ := firstOpcode(frames, serverpackets.OpcodeSystemMessage)
	if want := sysMsg(serverpackets.SystemMessageClanHallAwardedToClanS1, append(le32(serverpackets.SystemMessageParamText), []byte("B\x00i\x00d\x00d\x00e\x00r\x00s\x00\x00\x00")...)); string(award) != string(want) {
		t.Fatalf("award notice = % x, want % x", award, want)
	}
	header, _ := firstOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate)
	if got := pledgeHall(header); got != moonstone {
		t.Fatalf("clan header hall = %d, want %d", got, moonstone)
	}
	row := storedHall(t, w, moonstone)
	if row.owner != auctionClanID || !row.paid || row.paidUntil != end+7*dayMs || row.endDate != 0 {
		t.Fatalf("stored hall = %+v, want Bidders owning it, paid a week from the end %d", row, end)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids after the award = %+v, want none", bids)
	}
	if got := clanAdena(t, w, rivalClanID); got != 19_800_000 {
		t.Fatalf("losing clan's adena = %d, want 19800000", got)
	}
	if got := clanAdena(t, w, auctionClanID); got != 0 {
		t.Fatalf("winning clan's adena = %d, want 0", got)
	}
	if at := storedBidAt(t, w, auctionClanID); at != 0 {
		t.Fatalf("winner auction_bid_at = %d, want 0", at)
	}
	if at := storedBidAt(t, w, rivalClanID); at != 0 {
		t.Fatalf("loser auction_bid_at = %d, want 0", at)
	}

	// The lease falls due with an empty warehouse: a day's grace.
	w.srv.Advance(t, 7*24*time.Hour)
	due := drainFrames(t, w.c)
	wantMessages(t, "lease due", due, serverpackets.SystemMessageClanHallPaymentDueTomorrowS1)
	notice, _ := firstOpcode(due, serverpackets.OpcodeSystemMessage)
	if want := sysMsg(serverpackets.SystemMessageClanHallPaymentDueTomorrowS1, numberParam(500_000)); string(notice) != string(want) {
		t.Fatalf("lease notice = % x, want % x", notice, want)
	}
	if grace := storedHall(t, w, moonstone); grace.paid || grace.paidUntil != row.paidUntil+dayMs {
		t.Fatalf("stored hall in grace = %+v, want unpaid until %d", grace, row.paidUntil+dayMs)
	}

	// Still unpaid a day later: the hall is lost and back up for auction.
	w.srv.Advance(t, 24*time.Hour)
	lost := drainFrames(t, w.c)
	wantMessages(t, "hall lost", lost, serverpackets.SystemMessageClanHallFeeOverdueOwnershipLost)
	header, ok := firstOpcode(lost, serverpackets.OpcodePledgeShowInfoUpdate)
	if !ok || pledgeHall(header) != 0 {
		t.Fatalf("clan header after the loss: % x", opcodes(lost))
	}
	lostMs := row.paidUntil + dayMs
	if free := storedHall(t, w, moonstone); free.owner != 0 || free.paid || free.paidUntil != 0 || free.endDate != lostMs+7*dayMs {
		t.Fatalf("stored hall after the loss = %+v, want free, auction ending %d", free, lostMs+7*dayMs)
	}
	if v, _ := w.srv.Halls.View(moonstone); v.OwnerID != 0 {
		t.Fatal("hall still owned")
	}
}

// TestAuctionSaleEndsWithWinner: a sale's end pays the selling clan the
// highest bid minus 10% and its deposit back, and the hall goes to the
// highest bidder; the seller's members see their clan header lose it.
func TestAuctionSaleEndsWithWinner(t *testing.T) {
	t.Parallel()
	paid := time.Now().Add(72 * time.Hour).UnixMilli()
	end := time.Now().Add(time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, rivalBidAt: moonstone,
		halls: []hallRow{{
			id: moonstone, owner: auctionClanID, paid: true, paidUntil: paid,
			sellerBid: 30_000_000, sellerName: "Talker", sellerCln: "Bidders", endDate: end,
		}},
		bids: []bidRow{{moonstone, rivalClanID, "Rival", "Rivals", 35_000_000, 1}},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	w.srv.Advance(t, time.Hour+time.Minute)
	frames := drainFrames(t, w.c)
	header, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate)
	if !ok || pledgeHall(header) != 0 {
		t.Fatalf("seller's clan header: % x", opcodes(frames))
	}
	if got := clanAdena(t, w, auctionClanID); got != 32_000_000 {
		t.Fatalf("selling clan's adena = %d, want 31500000 + 500000", got)
	}
	row := storedHall(t, w, moonstone)
	if row.owner != rivalClanID || row.sellerName != "" || row.sellerCln != "" || row.sellerBid != 0 || row.endDate != 0 {
		t.Fatalf("stored hall = %+v, want Rivals owning it, no sale", row)
	}
}

// TestClanHallLeasePaid: a lease whose term ended while the server was
// down is paid at boot from the clan warehouse, and the term runs a week
// further.
func TestClanHallLeasePaid(t *testing.T) {
	t.Parallel()
	due := time.Now().Add(-time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, adena: 600_000,
		halls: []hallRow{{id: moonstone, owner: auctionClanID, paid: true, paidUntil: due, endDate: due + 7*dayMs}},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the lease is waited out on the test clock")
	}
	w.srv.Advance(t, 0)
	if got := clanAdena(t, w, auctionClanID); got != 100_000 {
		t.Fatalf("clan adena after the lease = %d, want 100000", got)
	}
	if row := storedHall(t, w, moonstone); !row.paid || row.paidUntil != due+7*dayMs || row.owner != auctionClanID {
		t.Fatalf("stored hall = %+v, want paid until %d", row, due+7*dayMs)
	}
}
