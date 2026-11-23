package clan

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// An auction names its clans by id, never by name, and a destroyed clan's
// bid is withdrawn with it (#5113). Moonstone Hall (22) is a Gludio hall
// with a lease of 500,000.
const auctionHall = 22

// bootAuctionDestruction boots the alliance world (Founder leading Knights,
// Rival leading Rivals, Loner seeded out of the world) with the clan halls
// loaded, stmts seeded, and a clan dissolving at once on request.
func bootAuctionDestruction(t *testing.T, stmts []string) *clanWorld {
	t.Helper()
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := clan.DefaultConfig()
	cfg.DissolveDays = 0
	return bootAllianceCast(t, []castMember{loner}, stmts,
		dissolutionPages(t), gameservertest.WithClanConfig(cfg), gameservertest.WithClanHalls(halls, decos))
}

// clanWarehouseAdena is the adena in clan clanID's warehouse, stored.
func clanWarehouseAdena(t *testing.T, w *clanWorld, clanID int32) int64 {
	t.Helper()
	w.srv.FlushItems(t)
	return queryInt(t, w, `SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = 57 AND loc = 'CLANWH'`, clanID)
}

// dissolveKnights has Founder dissolve Knights at once.
func dissolveKnights(t *testing.T, w *clanWorld) {
	t.Helper()
	wantDispersed(t, "leader", w.masterCommandBy(t, w.leader, "dissolve_clan"))
	if _, ok := w.srv.Clans.Table().Get(knightsClanID); ok {
		t.Fatal("Knights still stands after its dissolution")
	}
}

// TestAuctionDestroyedLoserRefundsNoNamesake: Knights (22,000,000) and
// Rivals (25,000,000) bid on free Moonstone Hall. Knights is destroyed and
// its bid withdrawn with it; Loner then founds a new clan named Knights.
// At the end Rivals takes the hall, and the namesake is neither refunded
// Knights' bid nor told who won: it never bid.
func TestAuctionDestroyedLoserRefundsNoNamesake(t *testing.T) {
	end := time.Now().Add(time.Hour).UnixMilli()
	w := bootAuctionDestruction(t, []string{
		`INSERT INTO clanhall (id, ownerId, paidUntil, paid, sellerBid, sellerName, sellerClanName, endDate)
			VALUES (22, 0, 0, 0, 0, '', '', ` + strconv.FormatInt(end, 10) + `)`,
		`INSERT INTO auctions (clanhall_id, bidder_name, clan_oid, clan_name, max_bid, time_bid) VALUES
			(22, 'Founder', ` + itoa(knightsClanID) + `, 'Knights', 22000000, 1),
			(22, 'Rival', ` + itoa(rivalsClanID) + `, 'Rivals', 25000000, 2)`,
		`UPDATE clan_data SET auction_bid_at = 22 WHERE clan_id IN (` + itoa(knightsClanID) + `, ` + itoa(rivalsClanID) + `)`,
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}

	dissolveKnights(t, w)
	v, _ := w.srv.Halls.View(auctionHall)
	if len(v.Bidders) != 1 || v.Bidders[0].ClanID != rivalsClanID || v.Highest == nil || v.Highest.ClanID != rivalsClanID {
		t.Fatalf("Moonstone Hall after Knights' destruction = %+v, want Rivals' bid alone", v)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM auctions WHERE clanhall_id = 22 AND clan_oid = ?`, knightsClanID); got != 0 {
		t.Fatalf("Knights' stored bids = %d, want 0", got)
	}
	if got := w.srv.Halls.BidAt(knightsClanID); got != 0 {
		t.Fatalf("Knights bids at %d, want nowhere", got)
	}

	lone := w.bringIn(t, loner)
	if _, ok := firstOpcode(w.masterCommandBy(t, lone, "create_clan Knights"), serverpackets.OpcodePledgeShowMemberListAll); !ok {
		t.Fatal("Loner founded no clan Knights")
	}
	namesake, ok := w.srv.Clans.Table().ByName("Knights")
	if !ok || namesake.ID() == knightsClanID {
		t.Fatalf("namesake Knights = %v, want a new clan", namesake)
	}
	drainFrames(t, lone)
	drainFrames(t, w.member)

	w.srv.Advance(t, time.Hour+time.Minute)
	if got := queryInt(t, w, `SELECT ownerId FROM clanhall WHERE id = 22`); got != int64(rivalsClanID) {
		t.Fatalf("Moonstone Hall owner = %#x, want Rivals", got)
	}
	if !slices.Contains(messages(t, drainFrames(t, w.member)), int(serverpackets.SystemMessageClanHallAwardedToClanS1)) {
		t.Fatal("Rivals was not told it won")
	}
	if got := messages(t, drainFrames(t, lone)); slices.Contains(got, int(serverpackets.SystemMessageClanHallAwardedToClanS1)) {
		t.Fatalf("namesake told of the award: messages %v", got)
	}
	if got := clanWarehouseAdena(t, w, namesake.ID()); got != 0 {
		t.Fatalf("namesake's adena = %d, want 0: it never bid", got)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM auctions WHERE clanhall_id = 22`); got != 0 {
		t.Fatalf("stored bids after the award = %d, want 0", got)
	}
}

// TestAuctionSaleWithADestroyedHighestBidder: Rivals sells Moonstone Hall,
// which it owns, and Knights alone bids 35,000,000. Knights is destroyed and
// its bid withdrawn with it, so at the end the sale found no buyer: Rivals
// is told the hall was not sold, keeps it, is paid nothing, and the sale is
// dropped.
func TestAuctionSaleWithADestroyedHighestBidder(t *testing.T) {
	end := time.Now().Add(time.Hour).UnixMilli()
	paid := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuctionDestruction(t, []string{
		`INSERT INTO clanhall (id, ownerId, paidUntil, paid, sellerBid, sellerName, sellerClanName, endDate)
			VALUES (22, ` + itoa(rivalsClanID) + `, ` + strconv.FormatInt(paid, 10) + `, 1, 30000000, 'Rival', 'Rivals', ` + strconv.FormatInt(end, 10) + `)`,
		`INSERT INTO auctions (clanhall_id, bidder_name, clan_oid, clan_name, max_bid, time_bid) VALUES
			(22, 'Founder', ` + itoa(knightsClanID) + `, 'Knights', 35000000, 1)`,
		`UPDATE clan_data SET auction_bid_at = 22 WHERE clan_id = ` + itoa(knightsClanID),
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}

	dissolveKnights(t, w)
	if v, _ := w.srv.Halls.View(auctionHall); len(v.Bidders) != 0 || v.Highest != nil || v.Seller == nil {
		t.Fatalf("Moonstone Hall after Knights' destruction = %+v, want the sale standing without bids", v)
	}
	drainFrames(t, w.member)

	w.srv.Advance(t, time.Hour+time.Minute)
	if got := messages(t, drainFrames(t, w.member)); !slices.Contains(got, int(serverpackets.SystemMessageClanHallNotSold)) {
		t.Fatalf("Rivals' messages at the end = %v, want CLANHALL_NOT_SOLD", got)
	}
	if got := queryInt(t, w, `SELECT ownerId FROM clanhall WHERE id = 22`); got != int64(rivalsClanID) {
		t.Fatalf("Moonstone Hall owner = %#x, want Rivals still", got)
	}
	if got := queryInt(t, w, `SELECT sellerBid + endDate FROM clanhall WHERE id = 22`); got != 0 {
		t.Fatalf("Moonstone Hall sale left stored (sellerBid + endDate = %d), want none", got)
	}
	if got := clanWarehouseAdena(t, w, rivalsClanID); got != 0 {
		t.Fatalf("Rivals' adena = %d, want 0: nobody bought the hall", got)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM auctions WHERE clanhall_id = 22`); got != 0 {
		t.Fatalf("stored bids = %d, want 0", got)
	}
}
