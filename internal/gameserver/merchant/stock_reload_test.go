package merchant

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/rs/zerolog"
)

// reloadLists is one list selling product.
func reloadLists(product buylist.Product) *buylist.Table {
	return buylist.NewTable([]buylist.List{{ID: product.BuyListID, NPCID: 30001, Products: []buylist.Product{product}}})
}

// A product read from the lists a reload replaced has no stock once the
// reload changed it: it shows none and cannot be sold, and the new
// product's stock is untouched. A product the reload left as it was keeps
// selling.
func TestStockReloadRefusesReplacedProducts(t *testing.T) {
	t.Parallel()
	old := buylist.Product{BuyListID: 1, ItemID: 17, Price: 1, RestockDelayMillis: 3600000, MaxCount: 5}
	fresh := old
	fresh.Price = 2
	s := NewStock(reloadLists(old), nil, nil, time.Now, zerolog.Nop())
	if err := s.Reload(context.Background(), reloadLists(fresh)); err != nil {
		t.Fatal(err)
	}
	if got := s.Count(old); got != 0 {
		t.Fatalf("replaced product count = %d, want 0", got)
	}
	if s.Decrease(old, 1) {
		t.Fatal("replaced product sold")
	}
	if got := s.Count(fresh); got != 5 {
		t.Fatalf("reloaded product count = %d, want 5", got)
	}

	if err := s.Reload(context.Background(), reloadLists(fresh)); err != nil {
		t.Fatal(err)
	}
	if !s.Decrease(fresh, 1) || s.Count(fresh) != 4 {
		t.Fatalf("unchanged product after a reload: count %d, want a sale to 4", s.Count(fresh))
	}
}

// Sales of both versions of a product race reloads flipping between them:
// under -race nothing races, a sale lands only on the version current at
// that moment, and the count never leaves its range.
func TestStockReloadRacesSales(t *testing.T) {
	t.Parallel()
	a := buylist.Product{BuyListID: 1, ItemID: 17, Price: 1, RestockDelayMillis: 3600000, MaxCount: 1000}
	b := a
	b.Price = 2
	lists := [2]*buylist.Table{reloadLists(a), reloadLists(b)}
	s := NewStock(lists[0], nil, nil, time.Now, zerolog.Nop())

	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 200 {
			if err := s.Reload(context.Background(), lists[(i+1)%2]); err != nil {
				t.Error(err)
				return
			}
		}
	})
	for _, p := range []buylist.Product{a, b} {
		wg.Go(func() {
			for range 200 {
				s.Decrease(p, 1)
				if n := s.Count(p); n < 0 || n > p.MaxCount {
					t.Errorf("count %d out of range", n)
					return
				}
			}
		})
	}
	wg.Wait()
	// The last reload brings a back.
	if s.Count(b) != 0 || s.Count(a) == 0 {
		t.Fatalf("after the last reload: stale count %d, current %d", s.Count(b), s.Count(a))
	}
}
