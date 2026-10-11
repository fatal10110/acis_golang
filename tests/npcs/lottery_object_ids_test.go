package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestLotteryPayoutOntoHeldAdenaTakesNoObjectID: a claimed prize joining
// the adena the player holds takes no object id from the allocator, while
// a prize starting the player's adena takes exactly one.
func TestLotteryPayoutOntoHeldAdenaTakesNoObjectID(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		adena   int32
		wantIDs int
	}{
		{name: "held adena", adena: 10000, wantIDs: 0},
		{name: "no adena", wantIDs: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			endDate := time.Now().Add(48 * time.Hour).UnixMilli()
			w, ids := bootLottery(t, seedRounds(t,
				[]any{2, 68, 11, 55000, 60000, 30000, 7000, 3000, endDate - 7*24*3600*1000, 1},
				[]any{3, 0, 0, 60000, 60000, 0, 0, 0, endDate, 0},
			), tt.adena, []heldTicket{{2, 68, 11}})
			w.talkTo(t, w.seller)
			w.loto(t, 24) // the ticket list, whose link claims the ticket

			before := w.srv.IssuedObjectIDs()
			w.loto(t, int(ids[0]))
			if got := w.srv.IssuedObjectIDs() - before; got != tt.wantIDs {
				t.Fatalf("object ids spent by the payout = %d, want %d", got, tt.wantIDs)
			}
			if got := w.adena(t); got != int(tt.adena)+30000 {
				t.Fatalf("adena after the claim = %d, want %d", got, tt.adena+30000)
			}
			if got := w.srv.PlayerInventory(t, w.player).ItemsByTemplateID(item.AdenaID); len(got) != 1 {
				t.Fatalf("adena stacks = %d, want 1", len(got))
			}
			if held := w.heldTickets(t); len(held) != 0 {
				t.Fatalf("tickets after the claim = %v, want none", held)
			}
		})
	}
}
