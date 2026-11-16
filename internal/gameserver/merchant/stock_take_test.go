package merchant

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/rs/zerolog"
)

// takeStock is a stock over two limited products of list 1, swords (2)
// and arrows (3), writing its rows inline to the returned store.
func takeStock(t *testing.T) (s *Stock, store *gapStore, swords, arrows buylist.Product) {
	t.Helper()
	swords = buylist.Product{BuyListID: 1, ItemID: 2369, Price: 1000, RestockDelayMillis: 3600000, MaxCount: 2}
	arrows = buylist.Product{BuyListID: 1, ItemID: 17, Price: 1, RestockDelayMillis: 3600000, MaxCount: 3}
	lists := buylist.NewTable([]buylist.List{{ID: 1, NPCID: 30001, Products: []buylist.Product{swords, arrows}}})
	store = &gapStore{rows: map[stockKey]StockRow{}}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s = NewStock(lists, store, nil, func() time.Time { return now }, zerolog.Nop())
	return s, store, swords, arrows
}

// A take is all or nothing (#4958, owner-approved fix): lots of one
// product are summed against its count, and a refused take leaves every
// count and row as it was. An accepted take saves each product's row once.
func TestStockTakeAllOrNothing(t *testing.T) {
	t.Parallel()
	s, store, swords, arrows := takeStock(t)

	if s.Take(Lot{swords, 1}, Lot{swords, 1}, Lot{swords, 1}) {
		t.Fatal("three swords taken from a stock of two")
	}
	if s.Take(Lot{arrows, 1}, Lot{swords, 2}, Lot{arrows, 3}) {
		t.Fatal("four arrows taken from a stock of three")
	}
	if s.Take(Lot{arrows, 0}) {
		t.Fatal("a lot of no units taken")
	}
	if s.Count(swords) != 2 || s.Count(arrows) != 3 || len(store.ops) != 0 {
		t.Fatalf("after refused takes: swords %d arrows %d writes %v, want 2/3/none", s.Count(swords), s.Count(arrows), store.ops)
	}

	if !s.Take(Lot{arrows, 1}, Lot{swords, 1}, Lot{arrows, 2}) {
		t.Fatal("take within the stock refused")
	}
	if s.Count(swords) != 1 || s.Count(arrows) != 0 {
		t.Fatalf("after the take: swords %d arrows %d, want 1/0", s.Count(swords), s.Count(arrows))
	}
	if len(store.ops) != 2 || store.rows[stockKey{1, 17}].Count != 0 || store.rows[stockKey{1, 2369}].Count != 1 {
		t.Fatalf("rows after the take = %v %+v, want one save each: arrows 0, swords 1", store.ops, store.rows)
	}
	if s.Take(Lot{swords, 1}, Lot{arrows, 1}) || s.Count(swords) != 1 {
		t.Fatalf("take past the sold-out arrows changed the swords: %d", s.Count(swords))
	}
}

// Return gives a take back: a product back at its full count ends its
// restock and deletes its row, as if never sold; one still sold down keeps
// its timer and row.
func TestStockReturnRestoresTheTake(t *testing.T) {
	t.Parallel()
	s, store, swords, arrows := takeStock(t)

	if !s.Take(Lot{swords, 2}) {
		t.Fatal("take refused")
	}
	s.Return(Lot{swords, 2})
	if s.Count(swords) != 2 || len(store.rows) != 0 || len(s.restock) != 0 {
		t.Fatalf("after the return: swords %d rows %+v timers %v, want 2, none, none", s.Count(swords), store.rows, s.restock)
	}
	if want := []string{"save", "delete"}; len(store.ops) != 2 || store.ops[0] != want[0] || store.ops[1] != want[1] {
		t.Fatalf("writes = %v, want %v", store.ops, want)
	}

	for range 2 {
		if !s.Take(Lot{arrows, 1}) {
			t.Fatal("take refused")
		}
	}
	s.Return(Lot{arrows, 1})
	if s.Count(arrows) != 2 || len(s.restock) != 1 || store.rows[stockKey{1, 17}].Count != 2 {
		t.Fatalf("partial return: arrows %d timers %v rows %+v, want 2 with the first sale's row kept", s.Count(arrows), s.restock, store.rows)
	}
	s.Return(Lot{arrows, 5})
	if s.Count(arrows) != 3 {
		t.Fatalf("return past the full count: arrows %d, want 3", s.Count(arrows))
	}
}

// Buyers racing for the last units each take all they ask for or nothing:
// under -race the units taken add up to exactly what the stock held.
func TestStockTakeRacesForLastUnits(t *testing.T) {
	t.Parallel()
	product := buylist.Product{BuyListID: 1, ItemID: 17, Price: 1, RestockDelayMillis: 3600000, MaxCount: 1000}
	s := NewStock(reloadLists(product), nil, nil, time.Now, zerolog.Nop())
	var mu sync.Mutex
	taken := 0
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				if s.Take(Lot{product, 7}) {
					mu.Lock()
					taken += 7
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	if taken != 994 || s.Count(product) != 6 {
		t.Fatalf("taken %d, left %d; want 994 taken and 6 left", taken, s.Count(product))
	}
}
