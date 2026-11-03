package clanhall

import "github.com/fatal10110/acis_golang/internal/gameserver/clan"

// All returns every hall, by id, as the auction pages show it.
func (hs *Halls) All() []HallView {
	if hs == nil {
		return nil
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	out := make([]HallView, 0, len(hs.order))
	for _, h := range hs.order {
		out = append(out, h.viewLocked())
	}
	return out
}

// AssignResult is how Assign ended.
type AssignResult int

const (
	// Assigned gave the hall to the clan.
	Assigned AssignResult = iota
	// AssignUnknown names no hall.
	AssignUnknown
	// AssignHallOwned found the hall owned already.
	AssignHallOwned
	// AssignClanOwns found the clan owning a hall already.
	AssignClanOwns
)

// Assign gives free hall hallID to cl, a clan owning no hall, as an
// auction won does (setOwnerLocked). The checks and the change run under
// one lock, so no other change of the hall's owner slips in between.
func (hs *Halls) Assign(hallID int32, cl *clan.Clan) AssignResult {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[hallID]
	switch {
	case !ok:
		return AssignUnknown
	case h.ownerID != 0:
		return AssignHallOwned
	case cl.HallID() > 0:
		return AssignClanOwns
	}
	hs.setOwnerLocked(h, cl)
	return Assigned
}

// Free takes hall hallID from its owner (freeLocked), reporting false for
// a hall it does not know or one already free.
func (hs *Halls) Free(hallID int32) bool {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[hallID]
	if !ok || h.ownerID == 0 {
		return false
	}
	hs.freeLocked(h)
	return true
}

// EndAuction ends hall hallID's auction now, as its end date does
// (endAuctionLocked), reporting false for a hall holding no auction.
func (hs *Halls) EndAuction(hallID int32) bool {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	h, ok := hs.byID[hallID]
	if !ok || h.auction == nil {
		return false
	}
	stopAuction(h.auction)
	hs.endAuctionLocked(h)
	return true
}
