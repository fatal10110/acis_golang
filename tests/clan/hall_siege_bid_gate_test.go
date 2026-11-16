package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestHallSiegeRefusesABiddingClan: Besiegers stands as a bidder on
// Moonstone Hall's auction. Its registration on Devastated Castle's siege
// is refused with 689, then the siege window, and nothing is registered.
// The other way round, once registered on the siege Besiegers may not bid:
// it could otherwise win both halls. Both gates are an owner-approved
// divergence (#5110).
func TestHallSiegeRefusesABiddingClan(t *testing.T) {
	t.Parallel()
	besiegers := itoa(besiegersClanID)
	w := bootHallSiege(t, hallSiegeSetup{seed: []string{
		"INSERT INTO clanhall (id, endDate) VALUES (22, " + weekAhead() + ")",
		"INSERT INTO auctions (clanhall_id, bidder_name, clan_oid, clan_name, max_bid, time_bid) VALUES (22, 'Attacker', " + besiegers + ", 'Besiegers', 21000000, 1)",
		"UPDATE clan_data SET auction_bid_at = 22 WHERE clan_id = " + besiegers,
	}})
	frames := w.register(t, 1)
	if got, want := sysMsgParams(t, frames), [][]string{{"689"}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("messages = %v, want %v", got, want)
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeSiegeInfo); !ok {
		t.Fatal("the refusal got no siege window")
	}
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers = %#x, want none", got)
	}
	if got := w.storedAttackers(t, devastatedCastle); len(got) != 0 {
		t.Fatalf("stored attackers = %#x, want none", got)
	}
}

// TestHallSiegeAttackerMayNotBid: Besiegers, registered on Devastated
// Castle's siege, bids on free Moonstone Hall and is refused; no adena is
// taken and no bid is listed (owner-approved divergence, #5110).
func TestHallSiegeAttackerMayNotBid(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{seed: []string{
		"INSERT INTO clanhall (id, endDate) VALUES (22, " + weekAhead() + ")",
	}})
	w.register(t, 1)
	if !slices.Equal(w.siege.Attackers(), []int32{besiegersClanID}) {
		t.Fatalf("attackers = %#x, want Besiegers", w.siege.Attackers())
	}
	cl, ok := w.srv.Clans.Table().Get(besiegersClanID)
	if !ok {
		t.Fatal("no Besiegers")
	}
	if got := w.srv.Halls.Bid(22, cl, "Attacker", 25_000_000); got != clanhall.BidNotAllowed {
		t.Fatalf("bid = %v, want refused", got)
	}
	if got := w.srv.Halls.BidAt(besiegersClanID); got != 0 {
		t.Fatalf("Besiegers bids at %d, want none", got)
	}
	if v, _ := w.srv.Halls.View(22); len(v.Bidders) != 0 {
		t.Fatalf("Moonstone Hall bids = %+v, want none", v.Bidders)
	}
}
