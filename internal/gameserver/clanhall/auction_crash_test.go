package clanhall

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// The tests below crash the server after every write that reaches the
// database while an auction moves adena, restart it from what landed, and
// check that no clan can then end up with more adena than it could have
// without the crash (#3367). After a restart, every auction whose end has
// passed ends, and every clan still holding a bid withdraws it, which is
// everything a clan can turn back into adena.

const (
	crashHall    = int32(22)
	crashSellers = int32(0x10000031)
	crashBidders = int32(0x10000032)
	crashRivals  = int32(0x10000033)
)

// crashDB is the database as a crash leaves it: the clanhall, auctions and
// auction_bid_at rows, and the adena each clan's warehouse holds.
type crashDB struct {
	halls map[int32]HallRow
	bids  map[[2]int32]BidRow
	bidAt map[int32]int32
	adena map[int32]int
}

func (d crashDB) clone() crashDB {
	return crashDB{halls: maps.Clone(d.halls), bids: maps.Clone(d.bids), bidAt: maps.Clone(d.bidAt), adena: maps.Clone(d.adena)}
}

// crashLog is every write in the order it reached the database; the first
// n of them are what a crash after the n-th leaves.
type crashLog struct {
	mu     sync.Mutex
	writes []func(*crashDB)
}

func (l *crashLog) add(w func(*crashDB)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writes = append(l.writes, w)
}

func (l *crashLog) replay(base crashDB, n int) crashDB {
	l.mu.Lock()
	defer l.mu.Unlock()
	db := base.clone()
	for _, w := range l.writes[:n] {
		w(&db)
	}
	return db
}

// crashStore loads base and logs every write.
type crashStore struct {
	base crashDB
	log  *crashLog
}

func (s *crashStore) LoadHalls(context.Context) ([]HallRow, error) {
	return slices.Collect(maps.Values(s.base.halls)), nil
}

func (s *crashStore) LoadBids(context.Context) ([]BidRow, error) {
	bids := slices.Collect(maps.Values(s.base.bids))
	slices.SortFunc(bids, func(a, b BidRow) int { return cmp.Compare(b.Bid, a.Bid) })
	return bids, nil
}

func (s *crashStore) LoadClanBids(context.Context) ([]ClanBid, error) {
	var out []ClanBid
	for clanID, hallID := range s.base.bidAt {
		out = append(out, ClanBid{ClanID: clanID, HallID: hallID})
	}
	return out, nil
}

func (s *crashStore) UpdateHall(_ context.Context, row HallRow) error {
	s.log.add(func(d *crashDB) { d.halls[row.ID] = row })
	return nil
}

func (s *crashStore) UpdateEndDate(_ context.Context, hallID int32, end int64) error {
	s.log.add(func(d *crashDB) {
		row := d.halls[hallID]
		row.EndDate = end
		d.halls[hallID] = row
	})
	return nil
}

func (s *crashStore) UpdateSale(_ context.Context, hallID int32, seller Seller, end int64) error {
	s.log.add(func(d *crashDB) {
		row := d.halls[hallID]
		row.SellerBid, row.SellerName, row.SellerClanName, row.EndDate = seller.Bid, seller.Name, seller.ClanName, end
		d.halls[hallID] = row
	})
	return nil
}

func (s *crashStore) SaveBid(_ context.Context, row BidRow) error {
	s.log.add(func(d *crashDB) { d.bids[[2]int32{row.HallID, row.ClanID}] = row })
	return nil
}

func (s *crashStore) DeleteBids(_ context.Context, hallID int32) error {
	s.log.add(func(d *crashDB) {
		maps.DeleteFunc(d.bids, func(k [2]int32, _ BidRow) bool { return k[0] == hallID })
	})
	return nil
}

func (s *crashStore) DeleteBid(_ context.Context, hallID, clanID int32) error {
	s.log.add(func(d *crashDB) { delete(d.bids, [2]int32{hallID, clanID}) })
	return nil
}

func (s *crashStore) SetClanBid(_ context.Context, clanID, hallID int32) error {
	s.log.add(func(d *crashDB) {
		if hallID == 0 {
			delete(d.bidAt, clanID)
		} else {
			d.bidAt[clanID] = hallID
		}
	})
	return nil
}

// crashBank holds each clan's warehouse adena and logs each move the
// worst way the item persistence could write it: a debit only when its
// landing runs, the latest it can reach the database; a credit the moment
// it is made, the earliest it can.
type crashBank struct {
	log  *crashLog
	mu   sync.Mutex
	held map[int32]int
}

func (b *crashBank) PayHallFee(clanID int32, adena int) bool {
	_, ok := b.TakeAdena(clanID, adena)
	return ok
}

func (b *crashBank) TakeAdena(clanID int32, adena int) (Landing, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.held[clanID] < adena {
		return nil, false
	}
	b.held[clanID] -= adena
	return func(context.Context) error {
		b.log.add(func(d *crashDB) { d.adena[clanID] -= adena })
		return nil
	}, true
}

func (b *crashBank) ReturnAdena(clanID int32, adena int) Landing {
	b.mu.Lock()
	b.held[clanID] += adena
	b.mu.Unlock()
	b.log.add(func(d *crashDB) { d.adena[clanID] += adena })
	return nil
}

// fifoLanes queues every job and runs none until drain, which runs them
// in the order they were queued.
type fifoLanes struct{ jobs []func() }

func (w *fifoLanes) Enqueue(_ int32, job func()) bool {
	w.jobs = append(w.jobs, job)
	return true
}

func (w *fifoLanes) drain() {
	for len(w.jobs) > 0 {
		job := w.jobs[0]
		w.jobs = w.jobs[1:]
		job()
	}
}

// crashClans is a fresh clan table: the sellers owning crashHall when
// owned is set, the bidders and their rivals, all level 2.
func crashClans(owned bool) *clan.Table {
	clans := clan.NewTable()
	clans.Restore(clan.Snapshot{Clans: []clan.Row{
		{ID: crashSellers, Name: "Sellers", Level: 2},
		{ID: crashBidders, Name: "Bidders", Level: 2},
		{ID: crashRivals, Name: "Rivals", Level: 2},
	}}, time.Now(), 1)
	if owned {
		clans.RestoreHalls([]clan.HallOwner{{HallID: crashHall, ClanID: crashSellers}}, nil)
	}
	return clans
}

func crashHallData(t *testing.T) *hallmodel.Table {
	t.Helper()
	h, err := hallmodel.NewHall(hallmodel.HallAttrs{
		ID: int(crashHall), Alias: fmt.Sprintf("hall_%d", crashHall), Name: "Hall", Description: "Hall", Town: "Town",
		AuctionMin: 1000, Lease: 500,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := hallmodel.NewTable([]*hallmodel.Hall{h})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// crashRun is one auction scenario: base is the database it starts from,
// at the clock start, and act drives the auction through halls, then lets
// the clock run through loop.
type crashRun struct {
	base  crashDB
	start time.Time
	act   func(t *testing.T, hs *Halls, clans *clan.Table, loop *sim.Inline)
}

// settle restarts the halls from db at the clock at, lets every auction
// whose end has passed end, has every clan withdraw the bid it still
// holds, and returns each clan's adena then.
func settle(t *testing.T, data *hallmodel.Table, db crashDB, at time.Time) map[int32]int {
	t.Helper()
	clans := crashClans(db.halls[crashHall].OwnerID == crashSellers)
	log := &crashLog{}
	hs := NewHalls(data, clans, nil, &crashStore{base: db, log: log}, nil, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	loop := sim.NewInline(at)
	bank := &crashBank{log: log, held: maps.Clone(db.adena)}
	hs.Start(loop.NewQueue("halls"), bank, nil, nil)
	loop.Advance(time.Second)
	for _, id := range []int32{crashSellers, crashBidders, crashRivals} {
		cl, _ := clans.Get(id)
		hs.CancelBid(cl)
	}
	return bank.held
}

// crashEverywhere runs r with its writes held, then checks the database a
// crash after each of them leaves: once settled after a restart, no clan
// holds more adena than it does settled from where the run started or
// from where it ended. It returns the adena settled from the end.
func crashEverywhere(t *testing.T, r crashRun) map[int32]int {
	t.Helper()
	data := crashHallData(t)
	log := &crashLog{}
	lanes := &fifoLanes{}
	clans := crashClans(r.base.halls[crashHall].OwnerID == crashSellers)
	hs := NewHalls(data, clans, nil, &crashStore{base: r.base, log: log}, lanes, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	loop := sim.NewInline(r.start)
	hs.Start(loop.NewQueue("halls"), &crashBank{log: log, held: maps.Clone(r.base.adena)}, nil, nil)
	r.act(t, hs, clans, loop)
	lanes.drain()

	restartAt := loop.Now().Add(time.Minute)
	n := len(log.writes)
	before := settle(t, data, log.replay(r.base, 0), restartAt)
	after := settle(t, data, log.replay(r.base, n), restartAt)
	for i := 1; i < n; i++ {
		got := settle(t, data, log.replay(r.base, i), restartAt)
		for _, id := range []int32{crashSellers, crashBidders, crashRivals} {
			if limit := max(before[id], after[id]); got[id] > limit {
				t.Errorf("crash after write %d of %d: clan %#x settles with %d adena, more than %d without the crash", i, n, id, got[id], limit)
			}
		}
	}
	return after
}

func crashBase(adena map[int32]int) crashDB {
	return crashDB{halls: map[int32]HallRow{}, bids: map[[2]int32]BidRow{}, bidAt: map[int32]int32{}, adena: adena}
}

// A bid, raised, then withdrawn.
func TestAuctionBidAndCancelSurviveACrashAnywhere(t *testing.T) {
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	base := crashBase(map[int32]int{crashBidders: 10_000})
	base.halls[crashHall] = HallRow{ID: crashHall, EndDate: start.Add(24 * time.Hour).UnixMilli()}
	got := crashEverywhere(t, crashRun{base: base, start: start, act: func(t *testing.T, hs *Halls, clans *clan.Table, _ *sim.Inline) {
		cl, _ := clans.Get(crashBidders)
		if r := hs.Bid(crashHall, cl, "Leader", 2000); r != BidPlaced {
			t.Fatalf("bid = %v, want placed", r)
		}
		if r := hs.Bid(crashHall, cl, "Leader", 3000); r != BidPlaced {
			t.Fatalf("raised bid = %v, want placed", r)
		}
		if !hs.CancelBid(cl) {
			t.Fatal("cancel refused")
		}
	}})
	if got[crashBidders] != 9_700 {
		t.Fatalf("bidders settle with %d adena, want 9700: 3000 bid, 2700 back", got[crashBidders])
	}
}

// A sale ends: the seller is paid the highest bid minus the tax and its
// deposit back, the losing bid comes back minus the tax, and the hall goes
// to the highest bidder.
func TestAuctionEndSurvivesACrashAnywhere(t *testing.T) {
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	base := crashBase(map[int32]int{})
	base.halls[crashHall] = HallRow{
		ID: crashHall, OwnerID: crashSellers, PaidUntil: start.Add(72 * time.Hour).UnixMilli(), Paid: true,
		SellerBid: 2000, SellerName: "Leader", SellerClanName: "Sellers", EndDate: start.Add(time.Hour).UnixMilli(),
	}
	base.bids[[2]int32{crashHall, crashBidders}] = BidRow{HallID: crashHall, ClanID: crashBidders, Name: "B", ClanName: "Bidders", Bid: 5000, Time: 1}
	base.bids[[2]int32{crashHall, crashRivals}] = BidRow{HallID: crashHall, ClanID: crashRivals, Name: "R", ClanName: "Rivals", Bid: 3000, Time: 2}
	base.bidAt[crashBidders], base.bidAt[crashRivals] = crashHall, crashHall
	got := crashEverywhere(t, crashRun{base: base, start: start, act: func(_ *testing.T, _ *Halls, _ *clan.Table, loop *sim.Inline) {
		loop.Advance(2 * time.Hour)
	}})
	if got[crashSellers] != 5000 || got[crashRivals] != 2700 || got[crashBidders] != 0 {
		t.Fatalf("settled adena = %v, want sellers 5000 (4500 and the 500 deposit), rivals 2700, bidders 0", got)
	}
}

// A sale withdrawn: the bidder gets its bid back minus the tax.
func TestAuctionSaleCancelSurvivesACrashAnywhere(t *testing.T) {
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	base := crashBase(map[int32]int{crashRivals: 5000})
	base.halls[crashHall] = HallRow{
		ID: crashHall, OwnerID: crashSellers, PaidUntil: start.Add(72 * time.Hour).UnixMilli(), Paid: true,
		SellerBid: 2000, SellerName: "Leader", SellerClanName: "Sellers", EndDate: start.Add(24 * time.Hour).UnixMilli(),
	}
	got := crashEverywhere(t, crashRun{base: base, start: start, act: func(t *testing.T, hs *Halls, clans *clan.Table, _ *sim.Inline) {
		rivals, _ := clans.Get(crashRivals)
		if r := hs.Bid(crashHall, rivals, "R", 3000); r != BidPlaced {
			t.Fatalf("bid = %v, want placed", r)
		}
		sellers, _ := clans.Get(crashSellers)
		if !hs.CancelSale(sellers) {
			t.Fatal("cancel sale refused")
		}
	}})
	if got[crashRivals] != 4700 {
		t.Fatalf("rivals settle with %d adena, want 4700: 3000 bid, 2700 back", got[crashRivals])
	}
}
