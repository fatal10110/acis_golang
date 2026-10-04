package commons

import (
	"slices"
	"sync"
	"testing"
)

// Swap hands every reader the new entries at once: a Get, Len or All
// running beside it sees one table or the other, never a mix.
func TestLookupSwapIsAtomicForReaders(t *testing.T) {
	key := func(v int) int { return v }
	old := NewLookup([]int{1, 2, 3}, key)
	fresh := NewLookup([]int{10, 20}, key)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				all := old.All()
				if !slices.Equal(all, []int{1, 2, 3}) && !slices.Equal(all, []int{10, 20}) {
					t.Errorf("All() = %v, a mix of both tables", all)
					return
				}
				old.Get(2)
				old.Len()
			}
		}()
	}
	old.Swap(fresh)
	close(stop)
	wg.Wait()

	if _, ok := old.Get(1); ok {
		t.Fatal("Get(1) found an entry of the replaced table")
	}
	if v, ok := old.Get(20); !ok || v != 20 || old.Len() != 2 {
		t.Fatalf("Get(20) = %d, %v, Len() = %d; want the new table", v, ok, old.Len())
	}
	if v, ok := fresh.Get(10); !ok || v != 10 {
		t.Fatal("the source table lost its entries")
	}
}
