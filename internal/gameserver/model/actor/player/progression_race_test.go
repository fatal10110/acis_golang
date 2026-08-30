package player

import (
	"sync"
	"testing"
)

// TestProgressionRewardDoesNotRaceSaveSnapshot reproduces the #1890 race:
// kill-reward application on a timer goroutine concurrent with the
// disconnect/autosave snapshot of the same progression fields.
func TestProgressionRewardDoesNotRaceSaveSnapshot(t *testing.T) {
	table, err := NewLevelTable(map[int]Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 100},
		3: {RequiredExpToLevelUp: 1_000_000_000},
	})
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}

	c := &Character{CharLevel: 1, Exp: 0, SP: 0}
	const workers = 4
	const iterations = 2_000

	var wg sync.WaitGroup
	wg.Add(workers * 2)
	for range workers {
		go func() {
			defer wg.Done()
			for range iterations {
				c.RewardExpAndSp(table, 1, 1)
			}
		}()
		go func() {
			defer wg.Done()
			for range iterations {
				_ = c.ProgressionValues()
			}
		}()
	}
	wg.Wait()
}
