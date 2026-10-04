package clanhall

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/rs/zerolog"
)

const (
	laneHallA = int32(22)
	laneHallB = int32(23)
	laneClan  = int32(0x10000007)
)

// laneStore loads the given rows and keeps the last auction_bid_at written
// per clan; the other writes do nothing.
type laneStore struct {
	HallStore
	rows     []HallRow
	bids     []BidRow
	clanBids []ClanBid
	mu       sync.Mutex
	bidAt    map[int32]int32
}

func (s *laneStore) LoadHalls(context.Context) ([]HallRow, error)      { return s.rows, nil }
func (s *laneStore) LoadBids(context.Context) ([]BidRow, error)        { return s.bids, nil }
func (s *laneStore) LoadClanBids(context.Context) ([]ClanBid, error)   { return s.clanBids, nil }
func (s *laneStore) UpdateEndDate(context.Context, int32, int64) error { return nil }
func (s *laneStore) UpdateHall(context.Context, HallRow) error         { return nil }
func (s *laneStore) SaveBid(context.Context, BidRow) error             { return nil }
func (s *laneStore) DeleteBid(context.Context, int32, int32) error     { return nil }
func (s *laneStore) DeleteBids(context.Context, int32) error           { return nil }

func (s *laneStore) SetClanBid(_ context.Context, clanID, hallID int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bidAt[clanID] = hallID
	return nil
}

// heldLanes queues each job on its owner's lane and runs nothing until
// drain, which empties the lanes in the order given, each lane in order:
// a lane named last stands for one that was backed up.
type heldLanes struct {
	lanes map[int32][]func()
}

func (w *heldLanes) Enqueue(ownerID int32, job func()) bool {
	w.lanes[ownerID] = append(w.lanes[ownerID], job)
	return true
}

func (w *heldLanes) drain(order ...int32) {
	for _, id := range order {
		for _, job := range w.lanes[id] {
			job()
		}
		delete(w.lanes, id)
	}
	rest := make([]int32, 0, len(w.lanes))
	for id := range w.lanes {
		rest = append(rest, id)
	}
	slices.Sort(rest)
	for _, id := range rest {
		for _, job := range w.lanes[id] {
			job()
		}
	}
	w.lanes = map[int32][]func(){}
}

// adenaBank pays every fee and takes every refund.
type adenaBank struct{}

func (adenaBank) PayHallFee(int32, int) bool { return true }
func (adenaBank) ReturnAdena(int32, int)     {}

// A clan cancelling its bid on hall A and bidding on hall B at once must
// end with auction_bid_at = B stored, however far hall A's lane is behind:
// a stored 0 would let it bid on a second hall after a restart.
func TestClanBidWriteStaysInOrderAcrossHallLanes(t *testing.T) {
	var halls []*hallmodel.Hall
	for _, id := range []int32{laneHallA, laneHallB} {
		h, err := hallmodel.NewHall(hallmodel.HallAttrs{
			ID: int(id), Alias: fmt.Sprintf("hall_%d", id), Name: "Hall", Description: "Hall", Town: "Town",
			AuctionMin: 1000, Deposit: 1000, Lease: 100,
		}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		halls = append(halls, h)
	}
	data, err := hallmodel.NewTable(halls)
	if err != nil {
		t.Fatal(err)
	}
	clans := clan.NewTable()
	clans.Restore(clan.Snapshot{Clans: []clan.Row{{ID: laneClan, Name: "Bidders", Level: 2}}}, time.Now(), 1)
	cl, _ := clans.Get(laneClan)

	end := time.Now().Add(24 * time.Hour).UnixMilli()
	store := &laneStore{
		rows:  []HallRow{{ID: laneHallA, EndDate: end}, {ID: laneHallB, EndDate: end}},
		bidAt: map[int32]int32{},
	}
	lanes := &heldLanes{lanes: map[int32][]func(){}}
	hs := NewHalls(data, clans, nil, store, lanes, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	hs.Start(nil, adenaBank{}, nil)

	if got := hs.Bid(laneHallA, cl, "Leader", 2000); got != BidPlaced {
		t.Fatalf("bid on hall A = %v, want placed", got)
	}
	if !hs.CancelBid(cl) {
		t.Fatal("cancel bid on hall A refused")
	}
	if got := hs.Bid(laneHallB, cl, "Leader", 2000); got != BidPlaced {
		t.Fatalf("bid on hall B = %v, want placed", got)
	}
	// Hall A's lane is the one backed up: it runs last.
	lanes.drain(laneHallB, laneClan, laneHallA)

	if got := store.bidAt[laneClan]; got != laneHallB {
		t.Fatalf("stored auction_bid_at = %d, want %d", got, laneHallB)
	}
	if got := hs.BidAt(laneClan); got != laneHallB {
		t.Fatalf("BidAt = %d, want %d", got, laneHallB)
	}
}
