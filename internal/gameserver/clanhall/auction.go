package clanhall

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// Seller is the clan leader who put an owned hall up for auction, and the
// minimum bid the clan asked.
type Seller struct {
	Name     string
	ClanName string
	Bid      int
}

// Bidder is one clan's bid on a hall: the bidder's name, the clan, the bid
// and when it was last raised, in Unix milliseconds.
type Bidder struct {
	ClanID   int32
	Name     string
	ClanName string
	Bid      int
	Time     int64
}

// auction is one hall's auction. bidders keeps the bids in the order they
// were first made; listed gives the order every view and the highest-bid
// search read them in.
type auction struct {
	endDate int64
	seller  *Seller
	bidders []*Bidder
	highest *Bidder
	// mostBidders is the most bids the auction ever held at once; it
	// sizes the listing order's table.
	mostBidders int
	// draft is the sale the owner set up and has not confirmed yet: the
	// sale it will register and the auction's end then.
	draft *saleDraft
	// timer ends the auction; gen counts the timers armed, so an end
	// whose timer was replaced does nothing.
	timer *sim.Timer
	gen   uint64
}

type saleDraft struct {
	seller  Seller
	endDate int64
}

func (a *auction) bidder(clanID int32) *Bidder {
	for _, b := range a.bidders {
		if b.ClanID == clanID {
			return b
		}
	}
	return nil
}

func (a *auction) add(b *Bidder) {
	a.bidders = append(a.bidders, b)
	a.mostBidders = max(a.mostBidders, len(a.bidders))
}

func (a *auction) remove(clanID int32) *Bidder {
	for i, b := range a.bidders {
		if b.ClanID == clanID {
			a.bidders = slices.Delete(a.bidders, i, i+1)
			return b
		}
	}
	return nil
}

// listed returns the bids in the order the reference's bid map iterates
// them: by the bucket each clan id falls in, a table of 16 doubled each
// time the bids reached three quarters of it, then by when each bid was
// first made. Past the first table the order within a bucket is an
// approximation: a grown table splits its buckets in an order of its own.
func (a *auction) listed() []*Bidder {
	size := 16
	for a.mostBidders >= size-size/4 {
		size *= 2
	}
	out := append([]*Bidder(nil), a.bidders...)
	slices.SortStableFunc(out, func(x, y *Bidder) int {
		return bucket(x.ClanID, size) - bucket(y.ClanID, size)
	})
	return out
}

// bucket is the bucket clan id id falls in in a table of size buckets.
func bucket(id int32, size int) int {
	h := uint32(id)
	return int((h ^ h>>16) & 0x7fffffff & uint32(size-1))
}

// recalculateHighest makes the first of the highest positive bids, in
// listing order, the highest.
func (a *auction) recalculateHighest() {
	var best *Bidder
	highest := 0
	for _, b := range a.listed() {
		if b.Bid > highest {
			best, highest = b, b.Bid
		}
	}
	a.highest = best
}

// minimumBid is the bid to beat: the hall's minimum, or the seller's when
// it asked more.
func (h *hall) minimumBid() int {
	if h.auction.seller == nil {
		return h.data.AuctionMin
	}
	return max(h.data.AuctionMin, h.auction.seller.Bid)
}

// listedLocked reports whether h is up for auction: free, or put up for
// sale by its owner.
func (h *hall) listedLocked() bool {
	return h.auction != nil && (h.ownerID == 0 || h.auction.seller != nil)
}

// startAuctionLocked arms h's auction to end at its end date. An end date
// already passed is moved a week from now and stored, and the auction
// ends at once.
func (hs *Halls) startAuctionLocked(h *hall) {
	a := h.auction
	now := hs.nowLocked()
	delay := int64(0)
	if a.endDate <= now {
		a.endDate = now + weekMs
		hallID, end := h.id(), a.endDate
		hs.write("store clan hall auction end", hallID, func(ctx context.Context, st HallStore) error {
			return st.UpdateEndDate(ctx, hallID, end)
		})
	} else {
		delay = a.endDate - now
	}
	hs.armAuctionLocked(h, delay)
}

func (hs *Halls) armAuctionLocked(h *hall, delayMs int64) {
	if hs.queue == nil {
		return
	}
	a := h.auction
	stopAuction(a)
	a.gen++
	gen := a.gen
	a.timer = hs.queue.After(time.Duration(max(delayMs, 0))*time.Millisecond, func() {
		hs.mu.Lock()
		defer hs.mu.Unlock()
		if a.gen != gen || a.timer == nil {
			return
		}
		a.timer = nil
		hs.endAuctionLocked(h)
	})
}

func stopAuction(a *auction) {
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

// resetAuctionLocked drops h's sale, highest bid and end date and stops
// its end.
func (hs *Halls) resetAuctionLocked(h *hall) {
	a := h.auction
	a.highest, a.seller, a.draft, a.endDate = nil, nil, nil, 0
	stopAuction(a)
}

// endAuctionLocked ends h's auction. Without a bid a free hall's auction
// starts over, and a hall for sale stays its owner's: the selling clan is
// told it found no buyer and the sale is dropped. Otherwise the selling
// clan, if any, is paid the highest bid minus the tax and its deposit back,
// and the hall goes to the highest bidder.
func (hs *Halls) endAuctionLocked(h *hall) {
	a := h.auction
	if a.highest == nil {
		if a.seller == nil {
			hs.startAuctionLocked(h)
			return
		}
		seller, _ := hs.clans.ByName(a.seller.ClanName)
		hs.tellClan(seller, Notice{Kind: NoticeNotSold})
		hs.resetAuctionLocked(h)
		hs.updateLocked(h)
		return
	}
	var payouts []credit
	if a.seller != nil {
		seller, _ := hs.clans.ByName(a.seller.ClanName)
		payouts = append(owedBack(seller, a.highest.Bid, true), owedBack(seller, h.data.Lease, false)...)
	}
	winner, _ := hs.clans.ByName(a.highest.ClanName)
	hs.setOwnerLocked(h, winner, payouts...)
}

// removeBidsLocked drops every bid on h, stored and live. Each bidding
// clan still standing no longer bids anywhere, gets its bid back minus the
// tax unless it is newOwner, and, with a newOwner, is told who won. The
// refunds, and payouts, are paid once the bids' delete has landed
// (writeThenCredit).
func (hs *Halls) removeBidsLocked(h *hall, newOwner *clan.Clan, payouts ...credit) {
	hallID := h.id()
	a := h.auction
	var refunds []credit
	for _, b := range a.listed() {
		cl, ok := hs.clans.ByName(b.ClanName)
		if !ok {
			continue
		}
		hs.setBidAtLocked(cl.ID(), 0)
		if cl != newOwner {
			refunds = append(refunds, owedBack(cl, b.Bid, true)...)
		}
		if newOwner != nil {
			hs.tellClan(cl, Notice{Kind: NoticeAwarded, Clan: newOwner.Name()})
		}
	}
	hs.writeThenCredit("remove clan hall bids", hallID, func(ctx context.Context, st HallStore) error {
		return st.DeleteBids(ctx, hallID)
	}, append(refunds, payouts...))
	a.bidders = nil
}

// BidResult is how a bid ended.
type BidResult int

const (
	// BidIgnored names no auction, or no clan: nothing is said.
	BidIgnored BidResult = iota
	// BidPlaced took the bid, or what it adds to the clan's earlier one,
	// from the clan's warehouse.
	BidPlaced
	// BidTooLow did not beat the minimum bid or the clan's earlier one.
	BidTooLow
	// BidNoAdena found too little adena in the clan's warehouse.
	BidNoAdena
	// BidClanTooLow is a clan below level 2.
	BidClanTooLow
	// BidNotAllowed is a clan owning a hall, or a hall not up for auction.
	BidNotAllowed
	// BidElsewhere is a clan that already bid on another hall.
	BidElsewhere
)

// Bid has player name of clan cl bid amount on hall hallID. A bid must
// beat the minimum bid and the clan's earlier bid, and the clan's
// warehouse must hold what it adds. The clan must be level 2 or higher,
// own no hall and have bid on no other hall, and the hall must be up for
// auction: the same gates that open the bid form, checked again as the
// form may be stale. A placed bid is the clan's leader's in the bid list
// and stored under name; the clan is recorded as bidding on the hall. The
// adena it takes is written ahead of the bid's row, in the same lane job,
// and a failed write of it stores no row: a stored bid is always paid.
func (hs *Halls) Bid(hallID int32, cl *clan.Clan, name string, amount int) BidResult {
	if hs == nil || cl == nil {
		return BidIgnored
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[hallID]
	if !ok || h.auction == nil {
		return BidIgnored
	}
	info := cl.Info()
	switch {
	case info.Level < 2:
		return BidClanTooLow
	case info.HallID > 0 || !h.listedLocked():
		return BidNotAllowed
	case hs.bidAt[cl.ID()] != 0 && hs.bidAt[cl.ID()] != hallID:
		return BidElsewhere
	}
	a := h.auction
	if amount <= h.minimumBid() {
		return BidTooLow
	}
	required := amount
	b := a.bidder(cl.ID())
	if b != nil {
		if amount <= b.Bid {
			return BidTooLow
		}
		required -= b.Bid
	}
	if hs.bank == nil {
		return BidNoAdena
	}
	paid, ok := hs.bank.TakeAdena(cl.ID(), required)
	if !ok {
		return BidNoAdena
	}
	now := hs.nowLocked()
	if b == nil {
		a.add(&Bidder{ClanID: cl.ID(), Name: info.LeaderName, ClanName: info.Name, Bid: amount, Time: now})
	} else {
		b.Bid, b.Time = amount, now
	}
	a.recalculateHighest()
	hs.setBidAtLocked(cl.ID(), hallID)
	row := BidRow{HallID: hallID, ClanID: cl.ID(), Name: name, ClanName: info.Name, Bid: amount, Time: now}
	hs.write("store clan hall bid", hallID, func(ctx context.Context, st HallStore) error {
		if err := land(ctx, paid); err != nil {
			return fmt.Errorf("write the adena bid: %w", err)
		}
		return st.SaveBid(ctx, row)
	})
	return BidPlaced
}

// CancelBid withdraws cl's bid on the hall it bid on, reporting false when
// that hall holds no auction. The bid comes back minus the tax, once its
// row is deleted (writeThenCredit), and the clan bids nowhere; a clan
// whose bid is not in the auction any more gets nothing.
func (hs *Halls) CancelBid(cl *clan.Clan) bool {
	if hs == nil || cl == nil {
		return false
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[hs.bidAt[cl.ID()]]
	if !ok || h.auction == nil {
		return false
	}
	a := h.auction
	b := a.remove(cl.ID())
	if b == nil {
		return true
	}
	hallID, clanID := h.id(), cl.ID()
	hs.writeThenCredit("remove clan hall bid", hallID, func(ctx context.Context, st HallStore) error {
		return st.DeleteBid(ctx, hallID, clanID)
	}, owedBack(cl, b.Bid, true))
	hs.setBidAtLocked(clanID, 0)
	if b == a.highest {
		a.recalculateHighest()
	}
	return true
}

// DraftSale sets up the sale of the hall cl owns: cl's leader selling it
// with a minimum bid of minBid, the auction running days days from now.
// The sale is registered only once ConfirmSale pays the deposit. It
// reports false when cl owns no hall with an auction.
func (hs *Halls) DraftSale(cl *clan.Clan, days, minBid int) (endDate int64, ok bool) {
	if hs == nil || cl == nil {
		return 0, false
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h := hs.byOwnerLocked(cl.ID())
	if h == nil || h.auction == nil {
		return 0, false
	}
	info := cl.Info()
	end := hs.nowLocked() + int64(days)*dayMs
	h.auction.draft = &saleDraft{seller: Seller{Name: info.LeaderName, ClanName: info.Name, Bid: minBid}, endDate: end}
	return end, true
}

// SaleResult is how a sale's confirmation ended.
type SaleResult int

const (
	// SaleIgnored found no sale set up, or one already registered:
	// nothing is said.
	SaleIgnored SaleResult = iota
	// SaleRegistered took the deposit and put the hall up for auction.
	SaleRegistered
	// SaleNoAdena found less than the deposit in the clan's warehouse.
	SaleNoAdena
)

// ConfirmSale registers the sale cl set up for the hall it owns: the
// deposit, the hall's lease, is taken from cl's warehouse, the sale and
// its end date are stored, and the auction runs until then. The deposit
// is written ahead of the sale, as a bid's adena is ahead of its row.
func (hs *Halls) ConfirmSale(cl *clan.Clan) SaleResult {
	if hs == nil || cl == nil {
		return SaleIgnored
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[cl.HallID()]
	if !ok || h.auction == nil || h.auction.draft == nil || h.auction.seller != nil {
		return SaleIgnored
	}
	a := h.auction
	if hs.bank == nil {
		return SaleNoAdena
	}
	paid, ok := hs.bank.TakeAdena(cl.ID(), h.data.Lease)
	if !ok {
		return SaleNoAdena
	}
	seller := a.draft.seller
	a.seller, a.endDate, a.draft = &seller, a.draft.endDate, nil
	hallID, end := h.id(), a.endDate
	hs.write("store clan hall sale", hallID, func(ctx context.Context, st HallStore) error {
		if err := land(ctx, paid); err != nil {
			return fmt.Errorf("write the deposit paid: %w", err)
		}
		return st.UpdateSale(ctx, hallID, seller, end)
	})
	hs.armAuctionLocked(h, end-hs.nowLocked())
	return SaleRegistered
}

// CancelSale withdraws the sale of the hall cl owns, reporting false when
// that hall holds no auction. The bidders get their bids back minus the
// tax, each told the hall went to the selling clan; the deposit is not
// returned. A hall not for sale is left as it is.
func (hs *Halls) CancelSale(cl *clan.Clan) bool {
	if hs == nil || cl == nil {
		return false
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[cl.HallID()]
	if !ok || h.auction == nil {
		return false
	}
	a := h.auction
	if a.seller == nil {
		return true
	}
	seller, _ := hs.clans.ByName(a.seller.ClanName)
	hs.removeBidsLocked(h, seller)
	hs.resetAuctionLocked(h)
	hs.updateLocked(h)
	return true
}

// BidAt is the hall clan clanID bid on, 0 for none.
func (hs *Halls) BidAt(clanID int32) int32 {
	if hs == nil {
		return 0
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return hs.bidAt[clanID]
}

// HallView is one hall and its auction as the auction pages show them.
type HallView struct {
	ID          int32
	Name        string
	Town        string
	Description string
	Size        int
	Lease       int
	AuctionMin  int
	OwnerID     int32
	// Auction reports whether the hall holds an auction; the fields below
	// are its.
	Auction bool
	EndDate int64
	MinBid  int
	// Seller is the registered sale, nil for none.
	Seller *Seller
	// Bidders are the bids in listing order.
	Bidders []Bidder
}

// Bid returns clan clanID's bid in v.
func (v HallView) Bid(clanID int32) (Bidder, bool) {
	for _, b := range v.Bidders {
		if b.ClanID == clanID {
			return b, true
		}
	}
	return Bidder{}, false
}

// View returns hall hallID as the auction pages show it.
func (hs *Halls) View(hallID int32) (HallView, bool) {
	if hs == nil {
		return HallView{}, false
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[hallID]
	if !ok {
		return HallView{}, false
	}
	return h.viewLocked(), true
}

// Auctionable returns the halls up for auction, by id: the free halls
// holding an auction and the halls their owners put up for sale.
func (hs *Halls) Auctionable() []HallView {
	if hs == nil {
		return nil
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	var out []HallView
	for _, h := range hs.order {
		if h.listedLocked() {
			out = append(out, h.viewLocked())
		}
	}
	return out
}

func (h *hall) viewLocked() HallView {
	v := HallView{
		ID: h.id(), Name: h.data.Name, Town: h.data.Town, Description: h.data.Description,
		Size: h.data.Size, Lease: h.data.Lease, AuctionMin: h.data.AuctionMin, OwnerID: h.ownerID,
	}
	a := h.auction
	if a == nil {
		return v
	}
	v.Auction, v.EndDate, v.MinBid = true, a.endDate, h.minimumBid()
	if a.seller != nil {
		s := *a.seller
		v.Seller = &s
	}
	for _, b := range a.listed() {
		v.Bidders = append(v.Bidders, *b)
	}
	return v
}
