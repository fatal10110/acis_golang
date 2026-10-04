package clanhall

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/rs/zerolog"
)

const (
	saleHall   = int32(22)
	sellerClan = int32(0x10000011)
	bidderClan = int32(0x10000012)
)

// refundBank pays every fee and records each refund per clan.
type refundBank struct{ returned map[int32]int }

func (b *refundBank) PayHallFee(int32, int) bool { return true }
func (b *refundBank) ReturnAdena(clanID int32, adena int) {
	b.returned[clanID] += adena
}

// clanTold records every notice per clan.
type clanTold struct{ told map[int32][]Notice }

func (n *clanTold) HallChanged(*clan.Clan) {}
func (n *clanTold) TellClan(cl *clan.Clan, notice Notice) {
	n.told[cl.ID()] = append(n.told[cl.ID()], notice)
}

// Withdrawing a sale gives each bidder its bid back minus 10% and tells it
// the hall went to the selling clan; the seller gets nothing back and is
// told nothing, and the hall keeps its owner with no sale and no end date.
func TestCancelSaleRefundsAndTellsBidders(t *testing.T) {
	h, err := hallmodel.NewHall(hallmodel.HallAttrs{
		ID: int(saleHall), Alias: fmt.Sprintf("hall_%d", saleHall), Name: "Hall", Description: "Hall", Town: "Town",
		AuctionMin: 20_000_000, Lease: 500_000,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := hallmodel.NewTable([]*hallmodel.Hall{h})
	if err != nil {
		t.Fatal(err)
	}
	clans := clan.NewTable()
	clans.Restore(clan.Snapshot{Clans: []clan.Row{
		{ID: sellerClan, Name: "Sellers", Level: 2}, {ID: bidderClan, Name: "Bidders", Level: 2},
	}}, time.Now(), 1)
	clans.RestoreHalls([]clan.HallOwner{{HallID: saleHall, ClanID: sellerClan}}, nil)
	seller, _ := clans.Get(sellerClan)

	now := time.Now().UnixMilli()
	store := &laneStore{
		rows: []HallRow{{
			ID: saleHall, OwnerID: sellerClan, PaidUntil: now + 72*3600_000, Paid: true,
			SellerBid: 30_000_000, SellerName: "Leader", SellerClanName: "Sellers", EndDate: now + 24*3600_000,
		}},
		bids:     []BidRow{{HallID: saleHall, ClanID: bidderClan, Name: "Rival", ClanName: "Bidders", Bid: 35_000_000, Time: 1}},
		clanBids: []ClanBid{{ClanID: bidderClan, HallID: saleHall}},
		bidAt:    map[int32]int32{bidderClan: saleHall},
	}
	hs := NewHalls(data, clans, nil, store, nil, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	bank := &refundBank{returned: map[int32]int{}}
	told := &clanTold{told: map[int32][]Notice{}}
	hs.Start(nil, bank, told)

	if !hs.CancelSale(seller) {
		t.Fatal("cancel sale refused")
	}
	if got := bank.returned[bidderClan]; got != 31_500_000 {
		t.Fatalf("bidder refund = %d, want 31500000", got)
	}
	if got, ok := bank.returned[sellerClan]; ok {
		t.Fatalf("seller got %d back, want nothing: the deposit is lost", got)
	}
	if got := told.told[bidderClan]; len(got) != 1 || got[0].Kind != NoticeAwarded || got[0].Clan != "Sellers" {
		t.Fatalf("bidder told %+v, want the hall awarded to Sellers", got)
	}
	if got := told.told[sellerClan]; len(got) != 0 {
		t.Fatalf("seller told %+v, want nothing", got)
	}
	if got := store.bidAt[bidderClan]; got != 0 {
		t.Fatalf("stored bidder auction_bid_at = %d, want 0", got)
	}
	v, _ := hs.View(saleHall)
	if v.OwnerID != sellerClan || v.Seller != nil || v.EndDate != 0 || len(v.Bidders) != 0 {
		t.Fatalf("hall after the cancel = %+v, want Sellers owning it, no sale, no end, no bids", v)
	}
}
