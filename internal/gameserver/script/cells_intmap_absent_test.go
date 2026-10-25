package script

import (
	"slices"
	"sync"
	"testing"
)

// TestIntMapCellSetIfAbsent stores a value only for an absent key: a
// present key keeps its value and its place in the walk order, and the
// caller gets the value the key has.
func TestIntMapCellSetIfAbsent(t *testing.T) {
	c := &IntMapCell[string]{}
	if v, stored := c.SetIfAbsent(17, "a"); !stored || v != "a" {
		t.Fatalf("SetIfAbsent(17) on an empty cell = %q, %v; want a, true", v, stored)
	}
	c.Set(1, "b")
	if v, stored := c.SetIfAbsent(17, "c"); stored || v != "a" {
		t.Fatalf("SetIfAbsent(17) on a present key = %q, %v; want a, false", v, stored)
	}
	// 17 and 1 share a bucket, walked in the order they were first stored.
	if got := c.Keys(); !slices.Equal(got, []int32{17, 1}) {
		t.Fatalf("keys = %v, want [17 1]", got)
	}
	if v, _ := c.Get(17); v != "a" {
		t.Fatalf("17 = %q after a refused store, want a", v)
	}
}

// TestIntMapCellSumsThroughCounterCells sums from many goroutines into
// counters stored with SetIfAbsent: no addition is lost, whichever caller
// stores a key's counter first.
func TestIntMapCellSumsThroughCounterCells(t *testing.T) {
	c := &IntMapCell[*IntCell]{}
	const workers, adds = 8, 500
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range adds {
				counter, _ := c.SetIfAbsent(int32((w+i)%3), &IntCell{})
				counter.Add(1)
			}
		}()
	}
	wg.Wait()
	total := 0
	for _, e := range c.Entries() {
		total += e.Value.Get()
	}
	if c.Len() != 3 || total != workers*adds {
		t.Fatalf("keys %d, total %d; want 3, %d", c.Len(), total, workers*adds)
	}
}
