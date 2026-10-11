package sevensigns

import "testing"

// TestWinningAndLosingCabal (SevenSignsManager.getWinningCabal,
// getLosingCabal): the cabal ahead on total score wins and the other one
// loses; a tie has neither.
func TestWinningAndLosingCabal(t *testing.T) {
	for _, tc := range []struct {
		name          string
		row           StatusRow
		winner, loser Cabal
	}{
		{"nothing scored", StatusRow{Cycle: 1, Period: Competition}, NoCabal, NoCabal},
		{"dawn ahead on stones", StatusRow{Cycle: 1, Period: Competition, DawnStoneScore: 3, DuskStoneScore: 1}, Dawn, Dusk},
		{"dusk ahead on stones", StatusRow{Cycle: 1, Period: Competition, DawnStoneScore: 1, DuskStoneScore: 3}, Dusk, Dawn},
		{"dusk ahead on the festival", StatusRow{Cycle: 1, Period: Competition, DuskFestivalScore: 5}, Dusk, Dawn},
		{"tie", StatusRow{Cycle: 1, Period: Competition, DawnStoneScore: 2, DuskStoneScore: 2}, NoCabal, NoCabal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := restoredHarness(t, tc.row)
			if got := h.state.WinningCabal(); got != tc.winner {
				t.Errorf("WinningCabal = %v, want %v", got, tc.winner)
			}
			if got := h.state.LosingCabal(); got != tc.loser {
				t.Errorf("LosingCabal = %v, want %v", got, tc.loser)
			}
		})
	}
}
