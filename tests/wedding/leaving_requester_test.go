package wedding

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestWeddingAcceptFromLeavingRequesterChargesNobody: Alice asks Bobby,
// then starts leaving the world before Bobby accepts. The price would be
// taken from her on Bobby's queue after her saves read her inventory, so
// she counts as offline: the accept is ignored as for a requester already
// gone, nobody pays, no couple forms, and her saved adena is untouched.
func TestWeddingAcceptFromLeavingRequesterChargesNobody(t *testing.T) {
	c := bootChapel(t, defaultChapel())
	c.talk(t, c.alice)
	c.bypass(t, c.alice, "AskWedding Bobby")
	drainFrames(t, c.bobby)
	alice := c.character(t, c.aliceID)
	alice.MarkDetaching()

	answer(t, c.bobby, true)
	if frames := drainFrames(t, c.bobby); len(frames) != 0 {
		t.Fatalf("Bobby after accepting = %x, want nothing", opcodes(frames))
	}
	if frames := drainFrames(t, c.alice); len(frames) != 0 {
		t.Fatalf("Alice after Bobby's accept = %x, want nothing", opcodes(frames))
	}
	if c.srv.Couples.CoupleID(c.aliceID) != 0 || c.srv.Couples.CoupleID(c.bobbyID) != 0 {
		t.Fatal("a leaving requester was married")
	}
	if c.adena(t, c.aliceID) != 2_000_000 || c.adena(t, c.bobbyID) != 2_000_000 {
		t.Fatalf("adena after the accept = %d and %d, want both untouched", c.adena(t, c.aliceID), c.adena(t, c.bobbyID))
	}
	c.srv.FlushItems(t)
	rows, err := c.srv.Items.ListByOwner(context.Background(), c.aliceID)
	if err != nil {
		t.Fatal(err)
	}
	saved := 0
	for _, row := range rows {
		if row.TemplateID == item.AdenaID {
			saved += row.Count
		}
	}
	if saved != 2_000_000 {
		t.Fatalf("Alice's saved adena = %d, want 2000000", saved)
	}
}
