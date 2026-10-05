package clanhall

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

const (
	// dayMs and weekMs are a day and a week in milliseconds: a grace day
	// for an unpaid lease, a paid lease's term and the length of an
	// auction restarted on its own.
	dayMs  = int64(24 * time.Hour / time.Millisecond)
	weekMs = 7 * dayMs
)

// HallRow is one clanhall row: the hall's owner and lease, and the sale
// its owner registered.
type HallRow struct {
	ID        int32
	OwnerID   int32
	PaidUntil int64
	Paid      bool
	// SellerBid, SellerName and SellerClanName are the registered sale's
	// minimum bid, the selling clan's leader and the selling clan; a row
	// missing either name holds no sale.
	SellerBid      int
	SellerName     string
	SellerClanName string
	// EndDate is when the hall's auction ends, in Unix milliseconds.
	EndDate int64
}

// BidRow is one auctions row: a clan's bid on a hall.
type BidRow struct {
	HallID int32
	ClanID int32
	// Name is the bidding player, ClanName the bidding clan.
	Name     string
	ClanName string
	Bid      int
	// Time is when the bid was last raised, in Unix milliseconds.
	Time int64
}

// ClanBid is one clan_data auction_bid_at: the hall a clan bid on.
type ClanBid struct {
	ClanID int32
	HallID int32
}

// HallStore persists the clan halls' owners, leases and auctions.
type HallStore interface {
	// LoadHalls returns every clanhall row.
	LoadHalls(ctx context.Context) ([]HallRow, error)
	// LoadBids returns every auctions row, the highest bid of each hall
	// first.
	LoadBids(ctx context.Context) ([]BidRow, error)
	// LoadClanBids returns the hall each clan bid on, for the clans that
	// did.
	LoadClanBids(ctx context.Context) ([]ClanBid, error)
	// UpdateHall writes every column of row.
	UpdateHall(ctx context.Context, row HallRow) error
	// UpdateEndDate writes hall hallID's auction end.
	UpdateEndDate(ctx context.Context, hallID int32, endDate int64) error
	// UpdateSale writes hall hallID's registered sale and auction end.
	UpdateSale(ctx context.Context, hallID int32, s Seller, endDate int64) error
	// SaveBid stores row, replacing the bidder name, bid and time of the
	// clan's earlier bid on the hall.
	SaveBid(ctx context.Context, row BidRow) error
	// DeleteBids drops every bid on hall hallID.
	DeleteBids(ctx context.Context, hallID int32) error
	// DeleteBid drops clan clanID's bid on hall hallID.
	DeleteBid(ctx context.Context, hallID, clanID int32) error
	// SetClanBid writes the hall clan clanID bid on, 0 for none.
	SetClanBid(ctx context.Context, clanID, hallID int32) error
}

// Clans resolves the clans owning, selling and bidding on the halls.
type Clans interface {
	Get(id int32) (*clan.Clan, bool)
	// ByName resolves a clan by name, ignoring case.
	ByName(name string) (*clan.Clan, bool)
}

// Bank moves adena in and out of a clan's warehouse.
type Bank interface {
	Treasury
	// ReturnAdena adds adena to clan clanID's warehouse, as much of it as
	// keeps the warehouse's adena within a 32-bit count.
	ReturnAdena(clanID int32, adena int)
}

// NoticeKind names a message every online member of a clan is told.
type NoticeKind int

const (
	// NoticeAwarded says the hall the clan bid on went to Notice.Clan.
	NoticeAwarded NoticeKind = iota + 1
	// NoticeNotSold says the clan's hall found no buyer.
	NoticeNotSold
	// NoticeFeeDue says the hall's lease, Notice.Lease, is unpaid and is
	// due by tomorrow.
	NoticeFeeDue
	// NoticeFeeOverdue says the lease is a week overdue and the hall is
	// lost.
	NoticeFeeOverdue
)

// Notice is one message to a clan's members.
type Notice struct {
	Kind  NoticeKind
	Clan  string
	Lease int
}

// Notifier tells the clans' members in the world what happened to their
// halls.
type Notifier interface {
	// HallChanged refreshes the clan header of cl's members, cl having
	// gained or lost a hall.
	HallChanged(cl *clan.Clan)
	// TellClan sends n to cl's members.
	TellClan(cl *clan.Clan, n Notice)
}

// Grounds acts on the halls' doors and grounds in the world. Its calls
// never call back into Halls.
type Grounds interface {
	// CloseDoors closes the doors named gates, a hall's gates list.
	CloseDoors(gates []string)
	// BanishForeigners teleports every player standing in hall hallID's
	// grounds who is not a member of clan clanID to a banish point of the
	// hall.
	BanishForeigners(hallID, clanID int32)
}

// Halls holds every clan hall's owner, lease and auction.
//
// mu guards every field below it and every hall and auction, and is held
// for a whole change, across the Bank, Notifier, Grounds, Clans and
// Functions calls it makes: none of them calls back into Halls.
type Halls struct {
	data   *hallmodel.Table
	clans  Clans
	fns    *Functions
	store  HallStore
	writes Writer
	log    zerolog.Logger

	mu    sync.Mutex
	byID  map[int32]*hall
	order []*hall
	// bidAt is the hall each clan bid on.
	bidAt   map[int32]int32
	queue   *sim.Queue
	bank    Bank
	notify  Notifier
	grounds Grounds
}

// hall is one clan hall's live state.
type hall struct {
	data      *hallmodel.Hall
	ownerID   int32
	paidUntil int64
	paid      bool
	// fee is the timer of the next lease payment; feeGen counts the timers
	// armed, so a payment whose timer was replaced charges nothing.
	fee    *sim.Timer
	feeGen uint64
	// auction is nil for a hall without a stored row or without a minimum
	// bid: a siegable hall.
	auction *auction
}

func (h *hall) id() int32 { return int32(h.data.ID) }

// NewHalls returns the halls of data, each free and without an auction;
// Restore then sets them from the database. fns loses a hall's functions
// when it changes hands. A nil store writes nothing; a nil writes runs
// each write at once.
func NewHalls(data *hallmodel.Table, clans Clans, fns *Functions, store HallStore, writes Writer, log zerolog.Logger) *Halls {
	hs := &Halls{
		data: data, clans: clans, fns: fns, store: store, writes: writes, log: log,
		byID: map[int32]*hall{}, bidAt: map[int32]int32{},
	}
	for _, d := range data.All() {
		h := &hall{data: d}
		hs.byID[h.id()] = h
		hs.order = append(hs.order, h)
	}
	// The halls are listed by id.
	slices.SortFunc(hs.order, func(a, b *hall) int { return int(a.id() - b.id()) })
	return hs
}

// Restore sets each hall from its stored row, its bids and the bids the
// clans made, once at boot after the clans are restored and before Start.
// A hall with a row and a minimum bid holds an auction: its stored sale
// when the row names both the selling clan and its leader, and its bids,
// the first one read standing highest. An owner that is not a clan leaves
// the hall free; the id factory already freed such rows.
func (hs *Halls) Restore(ctx context.Context) error {
	if hs == nil || hs.store == nil {
		return nil
	}
	rows, err := hs.store.LoadHalls(ctx)
	if err != nil {
		return err
	}
	bids, err := hs.store.LoadBids(ctx)
	if err != nil {
		return err
	}
	clanBids, err := hs.store.LoadClanBids(ctx)
	if err != nil {
		return err
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	for _, row := range rows {
		h, ok := hs.byID[row.ID]
		if !ok {
			continue
		}
		if h.data.AuctionMin > 0 {
			a := &auction{endDate: row.EndDate}
			if row.SellerName != "" && row.SellerClanName != "" {
				a.seller = &Seller{Name: row.SellerName, ClanName: row.SellerClanName, Bid: row.SellerBid}
			}
			h.auction = a
		}
		if row.OwnerID <= 0 {
			continue
		}
		if _, ok := hs.clans.Get(row.OwnerID); !ok {
			hs.log.Warn().Int32("hall_id", row.ID).Int32("clan_id", row.OwnerID).Msg("clanhall: owner is no clan; the hall stays free")
			continue
		}
		h.ownerID, h.paidUntil, h.paid = row.OwnerID, row.PaidUntil, row.Paid
	}
	for _, b := range bids {
		h, ok := hs.byID[b.HallID]
		if !ok || h.auction == nil {
			continue
		}
		bidder := &Bidder{ClanID: b.ClanID, Name: b.Name, ClanName: b.ClanName, Bid: b.Bid, Time: b.Time}
		if h.auction.bidder(b.ClanID) != nil {
			continue
		}
		if len(h.auction.bidders) == 0 {
			h.auction.highest = bidder
		}
		h.auction.add(bidder)
	}
	for _, cb := range clanBids {
		if cb.HallID > 0 {
			hs.bidAt[cb.ClanID] = cb.HallID
		}
	}
	return nil
}

// Start runs the halls on queue from now on: each owned hall's lease falls
// due when its paid term ends, at once when it already has, and each
// auction ends at its end date. An auction whose end date has passed is
// given a week from now, then ends at once. bank moves the clans' adena,
// notify tells their members and grounds closes the halls' doors and clears
// their grounds as they change hands.
func (hs *Halls) Start(queue *sim.Queue, bank Bank, notify Notifier, grounds Grounds) {
	if hs == nil {
		return
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.queue, hs.bank, hs.notify, hs.grounds = queue, bank, notify, grounds
	for _, h := range hs.order {
		if h.ownerID > 0 {
			hs.scheduleFeeLocked(h, h.paidUntil-hs.nowLocked())
		}
		if h.auction != nil {
			hs.startAuctionLocked(h)
		}
	}
}

// ByOwner is the hall clan clanID owns.
func (hs *Halls) ByOwner(clanID int32) (int32, bool) {
	if hs == nil || clanID == 0 {
		return 0, false
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if h := hs.byOwnerLocked(clanID); h != nil {
		return h.id(), true
	}
	return 0, false
}

func (hs *Halls) byOwnerLocked(clanID int32) *hall {
	for _, h := range hs.order {
		if h.ownerID == clanID {
			return h
		}
	}
	return nil
}

// SetOwner gives hall hallID to cl, reporting false for a hall it does not
// know. See setOwnerLocked.
func (hs *Halls) SetOwner(hallID int32, cl *clan.Clan) bool {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[hallID]
	if !ok {
		return false
	}
	hs.setOwnerLocked(h, cl)
	return true
}

// setOwnerLocked gives h to cl. The hall's auction first refunds its
// losing bidders, minus the tax, telling each bidding clan who won, and
// is reset. A nil cl, a winner gone since it bid, then restarts the
// auction and leaves the owner as it is. Otherwise the former owner loses
// the hall, the hall loses its functions and closes its doors, and cl owns
// it with a week's lease paid; both clans' headers are refreshed, the
// players of other clans are thrown out of its grounds and the row stored.
func (hs *Halls) setOwnerLocked(h *hall, cl *clan.Clan) {
	if a := h.auction; a != nil {
		hs.removeBidsLocked(h, cl)
		hs.resetAuctionLocked(h)
	}
	if cl == nil {
		if h.auction != nil {
			hs.startAuctionLocked(h)
		}
		return
	}
	if former, ok := hs.clans.Get(h.ownerID); ok && h.ownerID > 0 {
		former.SetHallID(0)
		hs.tellHallChanged(former)
	}
	hs.fns.RemoveAll(h.id())
	hs.closeDoorsLocked(h)
	cl.SetHallID(h.id())
	h.ownerID = cl.ID()
	h.paidUntil = hs.nowLocked() + weekMs
	h.paid = true
	hs.scheduleFeeLocked(h, h.paidUntil-hs.nowLocked())
	hs.tellHallChanged(cl)
	if hs.grounds != nil {
		hs.grounds.BanishForeigners(h.id(), h.ownerID)
	}
	hs.updateLocked(h)
}

// freeLocked takes h from its owner: the lease stops, the owner loses the
// hall and sees its header refreshed, the hall loses its functions and
// closes its doors, and its auction refunds its bidders, minus the tax,
// and starts over. The row is stored.
func (hs *Halls) freeLocked(h *hall) {
	hs.stopFeeLocked(h)
	if owner, ok := hs.clans.Get(h.ownerID); ok && h.ownerID > 0 {
		owner.SetHallID(0)
		hs.tellHallChanged(owner)
	}
	h.ownerID, h.paidUntil, h.paid = 0, 0, false
	hs.fns.RemoveAll(h.id())
	hs.closeDoorsLocked(h)
	if h.auction != nil {
		hs.removeBidsLocked(h, nil)
		hs.resetAuctionLocked(h)
		hs.startAuctionLocked(h)
	}
	hs.updateLocked(h)
}

// closeDoorsLocked closes h's gates.
func (hs *Halls) closeDoorsLocked(h *hall) {
	if hs.grounds != nil && len(h.data.Gates) > 0 {
		hs.grounds.CloseDoors(h.data.Gates)
	}
}

// scheduleFeeLocked arms h's next lease payment after delayMs, at once
// when it is not positive.
func (hs *Halls) scheduleFeeLocked(h *hall, delayMs int64) {
	if hs.queue == nil {
		return
	}
	hs.stopFeeLocked(h)
	h.feeGen++
	gen := h.feeGen
	h.fee = hs.queue.After(time.Duration(max(delayMs, 0))*time.Millisecond, func() { hs.payFee(h, gen) })
}

func (hs *Halls) stopFeeLocked(h *hall) {
	if h.fee != nil {
		h.fee.Stop()
		h.fee = nil
	}
}

// payFee charges h's weekly lease to its owner when its paid term ends.
// A free hall charges nothing, and an owner that is no clan any more loses
// the hall. A paid lease extends the term by a week. An unpaid one gives a
// day's grace, the owner's members told the lease is due, then, unpaid
// again, takes the hall, the members told it is lost. gen is the
// generation the payment's timer was armed with.
func (hs *Halls) payFee(h *hall, gen uint64) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if h.feeGen != gen || h.fee == nil {
		return
	}
	h.fee = nil
	if h.ownerID == 0 {
		return
	}
	owner, ok := hs.clans.Get(h.ownerID)
	if !ok {
		hs.freeLocked(h)
		return
	}
	lease := h.data.Lease
	switch {
	case lease <= 0 || (hs.bank != nil && hs.bank.PayHallFee(owner.ID(), lease)):
		hs.scheduleFeeLocked(h, weekMs)
		h.paidUntil += weekMs
		h.paid = true
		hs.updateLocked(h)
	case h.paid:
		hs.scheduleFeeLocked(h, dayMs)
		h.paidUntil += dayMs
		h.paid = false
		hs.updateLocked(h)
		hs.tellClan(owner, Notice{Kind: NoticeFeeDue, Lease: lease})
	default:
		hs.freeLocked(h)
		hs.tellClan(owner, Notice{Kind: NoticeFeeOverdue})
	}
}

// Now is the time on the clock the halls run on, in Unix milliseconds.
func (hs *Halls) Now() int64 {
	if hs == nil {
		return time.Now().UnixMilli()
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return hs.nowLocked()
}

func (hs *Halls) nowLocked() int64 {
	if hs.queue == nil {
		return time.Now().UnixMilli()
	}
	return hs.queue.Now().UnixMilli()
}

func (hs *Halls) tellHallChanged(cl *clan.Clan) {
	if hs.notify != nil {
		hs.notify.HallChanged(cl)
	}
}

func (hs *Halls) tellClan(cl *clan.Clan, n Notice) {
	if hs.notify != nil && cl != nil {
		hs.notify.TellClan(cl, n)
	}
}

// returnAdenaLocked gives cl back adena, minus the 10% tax when taxed,
// truncated. A nil cl, a clan gone since, gets nothing.
func (hs *Halls) returnAdenaLocked(cl *clan.Clan, adena int, taxed bool) {
	if cl == nil || hs.bank == nil {
		return
	}
	if taxed {
		adena = TaxedRefund(adena)
	}
	hs.bank.ReturnAdena(cl.ID(), adena)
}

// TaxedRefund is what a bid of adena returns once the 10% tax is taken:
// adena times 0.9, truncated.
func TaxedRefund(adena int) int {
	return int(float64(adena) * 0.9)
}

// updateLocked stores every column of h's row.
func (hs *Halls) updateLocked(h *hall) {
	row := HallRow{ID: h.id(), OwnerID: h.ownerID, PaidUntil: h.paidUntil, Paid: h.paid}
	if a := h.auction; a != nil {
		if a.seller != nil {
			row.SellerBid, row.SellerName, row.SellerClanName = a.seller.Bid, a.seller.Name, a.seller.ClanName
		}
		row.EndDate = a.endDate
	}
	hs.write("store clan hall", row.ID, func(ctx context.Context, st HallStore) error {
		return st.UpdateHall(ctx, row)
	})
}

// setBidAtLocked records the hall clan clanID bid on, 0 for none, and
// stores it on the clan's row. The write goes on the clan's lane, not a
// hall's: a clan's bids on two halls would otherwise queue its writes on
// two lanes that run concurrently, and an older 0 could land last.
func (hs *Halls) setBidAtLocked(clanID, at int32) {
	if at == 0 {
		delete(hs.bidAt, clanID)
	} else {
		hs.bidAt[clanID] = at
	}
	hs.writeOn("store clan auction bid", "clan_id", clanID, func(ctx context.Context, st HallStore) error {
		return st.SetClanBid(ctx, clanID, at)
	})
}

// write queues fn on hallID's persistence lane, or runs it at once
// without a writer. Each write gets taskTimeout.
func (hs *Halls) write(what string, hallID int32, fn func(context.Context, HallStore) error) {
	hs.writeOn(what, "hall_id", hallID, fn)
}

// writeOn queues fn on owner's persistence lane, owner being the id named
// key, or runs it at once without a writer.
func (hs *Halls) writeOn(what, key string, owner int32, fn func(context.Context, HallStore) error) {
	store, log := hs.store, hs.log
	if store == nil {
		return
	}
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32(key, owner).Msg("clanhall: " + what)
		}
	}
	if hs.writes == nil {
		job()
		return
	}
	if !hs.writes.Enqueue(owner, job) {
		log.Error().Int32(key, owner).Msg("clanhall: " + what + ": write dropped")
	}
}
