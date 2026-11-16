package npcs

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAuctionSiegeAttackerMayNotBid: Bidders attacks Devastated Castle's
// siege, so the auctioneer refuses it the bid form, with the list's first
// page and CANNOT_PARTICIPATE_IN_AUCTION, and the halls refuse its bid: it
// could win both halls (owner-approved divergence, #5110). No
// adena is taken and no bid is stored.
func TestAuctionSiegeAttackerMayNotBid(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	setup := auctionSetup{clanLevel: 2, adena: 30_000_000, halls: everyHall(end)}
	noop := func() {}
	w := bootAuction(t, setup, gameservertest.WithHallSiegeScript(devastatedCastle, clanhall.SiegeScript{
		Winner: func() int32 { return 0 }, SpawnNPCs: noop, UnspawnNPCs: noop,
	}))
	hs, ok := w.srv.Halls.Siege(devastatedCastle)
	if !ok || !hs.AddAttacker(auctionClanID) {
		t.Fatal("Bidders did not join Devastated Castle's siege")
	}
	w.open(t)
	w.page(t, "list")
	w.page(t, "bidding 22")
	frames := w.bypass(t, "bid1 22")
	if page := pageIn(t, frames); !strings.Contains(page, "_bidding 22\">Moonstone Hall") {
		t.Fatalf("refusal page is not the list:\n%s", page)
	}
	wantMessages(t, "bid1 22", frames, serverpackets.SystemMessageCannotParticipateInAuction)
	// The refused form leaves no bid link to follow, so the bid itself goes
	// straight to the halls.
	cl, ok := w.srv.Clans.Table().Get(auctionClanID)
	if !ok {
		t.Fatal("no Bidders")
	}
	if got := w.srv.Halls.Bid(moonstone, cl, "Talker", 25_000_000); got != clanhall.BidNotAllowed {
		t.Fatalf("bid = %v, want refused", got)
	}
	if got := clanAdena(t, w, auctionClanID); got != 30_000_000 {
		t.Fatalf("clan adena = %d, want 30000000 untouched", got)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids = %+v, want none", bids)
	}
}
