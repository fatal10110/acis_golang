package sevensigns

import (
	"context"
	"testing"
	"time"
)

// TestSavePlayerWritesOneRow pins SavePlayer: it writes only the given
// player's current row, never the status row or another sign-up, and writes
// nothing for a player who has not signed up.
func TestSavePlayerWritesOneRow(t *testing.T) {
	h := newStateHarness(t, at(2026, time.August, 26, 12, 0, time.UTC))
	h.store.players = []PlayerRow{
		{ObjectID: 1, Cabal: Dawn, Seal: Avarice},
		{ObjectID: 2, Cabal: Dusk, Seal: Gnosis},
	}
	ctx := context.Background()
	if err := h.state.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.state.AddPlayerStoneContrib(2, 4, 0, 0, 1000000); !ok {
		t.Fatal("turn-in refused")
	}
	if err := h.state.SavePlayer(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := h.state.SavePlayer(ctx, 3); err != nil {
		t.Fatal(err)
	}
	want := PlayerRow{ObjectID: 2, Cabal: Dusk, Seal: Gnosis, BlueStones: 4, AncientAdena: 12, ContributionScore: 12}
	if len(h.store.playerSaves) != 1 || len(h.store.playerSaves[0]) != 1 || h.store.playerSaves[0][0] != want {
		t.Fatalf("player saves = %+v, want one save of %+v", h.store.playerSaves, want)
	}
	if len(h.store.saves) != 0 {
		t.Fatalf("status saves = %+v, want none", h.store.saves)
	}
}
