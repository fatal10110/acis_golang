// Package merchant runs the NPC shop economy over the static buylists: the
// limited-stock counters and their restock timers, and the buy and try-on
// transactions a merchant's windows lead to.
package merchant

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// RestockTick is how often the stock checks for products due a restock.
const RestockTick = time.Second

// StockRow is one buylists row: a limited product's saved count and the
// time it restocks.
type StockRow struct {
	ListID      int
	ItemID      int32
	Count       int
	NextRestock time.Time
}

// StockStore reads and writes the buylists rows.
type StockStore interface {
	LoadStock(ctx context.Context) ([]StockRow, error)
	SaveStock(ctx context.Context, row StockRow) error
	DeleteStock(ctx context.Context, listID int, itemID int32) error
}

type stockKey struct {
	list int
	item int32
}

// Stock owns the current count of every limited product and the restock
// timers of the ones sold since their last restock. A product joins the
// restock timers on its first sale after a restock; that sale saves its
// row, and later sales before the restock change only the count in memory.
// The restock puts the product back to its full count and deletes the row.
type Stock struct {
	store  StockStore
	worker *persist.Worker
	now    func() time.Time
	log    zerolog.Logger

	mu       sync.Mutex
	products map[stockKey]buylist.Product
	counts   map[stockKey]int
	restock  map[stockKey]time.Time
}

// NewStock starts every limited product of lists at its full count. Row
// writes run on worker, one lane per buylist; a nil worker writes inline
// while holding the stock's lock, which only a test without a database
// should rely on.
func NewStock(lists *buylist.Table, store StockStore, worker *persist.Worker, now func() time.Time, log zerolog.Logger) *Stock {
	s := &Stock{
		store:    store,
		worker:   worker,
		now:      now,
		log:      log,
		products: map[stockKey]buylist.Product{},
		counts:   map[stockKey]int{},
		restock:  map[stockKey]time.Time{},
	}
	if lists != nil {
		lists.Each(func(l buylist.List) {
			for _, p := range l.Products {
				if p.LimitedStock() {
					key := stockKey{l.ID, p.ItemID}
					s.products[key] = p
					s.counts[key] = p.MaxCount
				}
			}
		})
	}
	return s
}

// Restore applies the saved rows: a row whose restock is still ahead sets
// its product's count and restock time, and any other row is deleted,
// leaving its product at its full count. A row naming no limited product
// is ignored.
func (s *Stock) Restore(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	rows, err := s.store.LoadStock(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, row := range rows {
		key := stockKey{row.ListID, row.ItemID}
		p, ok := s.products[key]
		if !ok {
			continue
		}
		if row.NextRestock.After(now) {
			s.mu.Lock()
			s.counts[key] = row.Count
			if _, pending := s.restock[key]; !pending {
				s.restock[key] = row.NextRestock
			}
			s.mu.Unlock()
			continue
		}
		s.mu.Lock()
		s.counts[key] = p.MaxCount
		s.mu.Unlock()
		if err := s.store.DeleteStock(ctx, row.ListID, row.ItemID); err != nil {
			return err
		}
	}
	return nil
}

// Count returns p's current count: 0 for a product without a limited
// stock, and never below 0.
func (s *Stock) Count(p buylist.Product) int {
	if !p.LimitedStock() {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return max(s.counts[stockKey{p.BuyListID, p.ItemID}], 0)
}

// Decrease takes n off p's count and reports whether the count stayed at
// 0 or above. The count is lowered either way, so an oversold count reads
// as 0 until the restock. A successful decrease on a product not waiting
// for a restock starts its restock timer and saves its row.
func (s *Stock) Decrease(p buylist.Product, n int) bool {
	if !p.LimitedStock() {
		return false
	}
	key := stockKey{p.BuyListID, p.ItemID}
	s.mu.Lock()
	count := s.counts[key] - n
	s.counts[key] = count
	if count < 0 {
		s.mu.Unlock()
		return false
	}
	if _, pending := s.restock[key]; pending {
		s.mu.Unlock()
		return true
	}
	next := s.now().Add(time.Duration(p.RestockDelayMillis) * time.Millisecond)
	s.restock[key] = next
	row := StockRow{ListID: p.BuyListID, ItemID: p.ItemID, Count: count, NextRestock: next}
	s.write(p.BuyListID, func(ctx context.Context) error { return s.store.SaveStock(ctx, row) })
	s.mu.Unlock()
	return true
}

// Restock puts every product whose restock time has come back to its full
// count, ends its timer and deletes its row.
func (s *Stock) Restock() {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, at := range s.restock {
		if now.Before(at) {
			continue
		}
		s.counts[key] = s.products[key].MaxCount
		delete(s.restock, key)
		s.write(key.list, func(ctx context.Context) error { return s.store.DeleteStock(ctx, key.list, key.item) })
	}
}

// Start runs Restock every RestockTick until the returned ticker stops.
func (s *Stock) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(RestockTick, s.Restock, log)
}

// write queues one row write on listID's lane. The caller holds s.mu, so
// the writes of one product reach the lane in the order its count and
// restock timer changed: a restock's delete and the next sale's save can
// not swap. Enqueue only appends to the lane; a nil worker runs the write
// inline, still under s.mu.
func (s *Stock) write(listID int, op func(context.Context) error) {
	if s.store == nil {
		return
	}
	s.worker.Enqueue(int32(listID), func() {
		if err := op(context.Background()); err != nil {
			s.log.Error().Err(err).Int("buylist", listID).Msg("merchant: write buylist stock row")
		}
	})
}
