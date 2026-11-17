package derby

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

// historyStore is a fakeStore that also holds race records.
type historyStore struct {
	fakeStore
	history []History
}

func (s *historyStore) LoadHistory(context.Context) ([]History, error) { return s.history, nil }

// heldItem holds one item, read as a ticket.
type heldItem struct{ item Ticket }

func (heldItem) Tickets() []Ticket { return nil }
func (h heldItem) Item(objectID int32) (Ticket, bool) {
	return h.item, objectID == h.item.ObjectID
}

// TestOnlyRaceTicketsAreReadAsTickets pins the template check: an item
// that is not a race ticket, though it carries the race, lane and price of
// a winning one, is shown and cashed as an item not held.
func TestOnlyRaceTicketsAreReadAsTickets(t *testing.T) {
	t.Parallel()
	store := &historyStore{history: []History{{RaceID: 1, First: 4, Second: 1, OddRate: 1.25}}}
	track, err := New(context.Background(), store, testTemplates(), &fakeIDs{}, nil, fakeRand{}, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	ticket := Ticket{ObjectID: 7, ItemID: TicketItemID, Race: 1, Lane: 5, Hundreds: 10}
	var picks Picks
	reply := track.Bypass(noPages{}, 30995, 1, &picks, heldItem{ticket}, "CalculateWin 7")
	if reply.Redeem == nil || *reply.Redeem != (Redemption{ObjectID: 7, Payout: 1250}) {
		t.Fatalf("cashing the winning ticket = %+v, want 1250 adena for object 7", reply)
	}

	other := ticket
	other.ItemID = 57
	for _, command := range []string{"ShowTicket", "CalculateWin"} {
		got := track.Bypass(noPages{}, 30995, 1, &picks, heldItem{other}, command+" 7")
		want := track.Bypass(noPages{}, 30995, 1, &picks, heldItem{other}, command+" 8")
		if got != want || got.Redeem != nil {
			t.Fatalf("%s of a held non-ticket = %+v, want %+v as for an item not held", command, got, want)
		}
	}
}
