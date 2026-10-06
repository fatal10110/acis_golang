package clan

import (
	"strconv"
	"testing"
	"time"
)

// TestRefreshLadderRanksCurrentReputation restores 101 clans of level 5
// with positive reputation and one in debt, then moves the scores: the
// refresh drops every rank, ranks only the 99 best current scores (equal
// scores by clan id), and leaves the rest, the indebted clan included,
// unranked.
func TestRefreshLadderRanksCurrentReputation(t *testing.T) {
	var rows []Row
	for id := int32(1); id <= 101; id++ {
		rows = append(rows, Row{ID: id, Name: "Clan" + strconv.Itoa(int(id)), Level: 5, Reputation: int(id)})
	}
	rows = append(rows, Row{ID: 102, Name: "Indebted", Level: 5, Reputation: -5})
	table := NewTable()
	table.Restore(Snapshot{Clans: rows}, time.UnixMilli(0), 1)
	rank := func(id int32) int {
		cl, _ := table.Get(id)
		return cl.Info().Rank
	}
	if rank(101) != 1 || rank(3) != 99 || rank(2) != 0 {
		t.Fatalf("restored ranks of 101, 3, 2 = %d, %d, %d; want 1, 99, 0", rank(101), rank(3), rank(2))
	}

	// Clan 1 jumps to the top; clans 50 and 51 tie at 1000.
	for id, rep := range map[int32]int{1: 5000, 50: 1000, 51: 1000} {
		cl, _ := table.Get(id)
		cl.mu.Lock()
		cl.reputation = rep
		cl.mu.Unlock()
	}
	table.RefreshLadder()
	for _, tc := range []struct {
		id   int32
		want int
	}{
		{1, 1}, {50, 2}, {51, 3}, {101, 4}, {5, 98}, {4, 99}, {3, 0}, {2, 0}, {102, 0},
	} {
		if got := rank(tc.id); got != tc.want {
			t.Errorf("clan %d rank = %d, want %d", tc.id, got, tc.want)
		}
	}
}
