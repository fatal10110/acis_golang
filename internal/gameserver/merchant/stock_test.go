package merchant

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/rs/zerolog"
)

// gapStore records the buylists row writes in the order they reach the
// store and keeps the table they leave behind. Before recording a delete it
// runs onDelete, the moment a concurrent buyer could act if the write ran
// outside the stock's lock.
type gapStore struct {
	ops      []string
	rows     map[stockKey]StockRow
	onDelete func()
}

func (g *gapStore) LoadStock(context.Context) ([]StockRow, error) { return nil, nil }

func (g *gapStore) SaveStock(_ context.Context, row StockRow) error {
	g.ops = append(g.ops, "save")
	g.rows[stockKey{row.ListID, row.ItemID}] = row
	return nil
}

func (g *gapStore) DeleteStock(_ context.Context, listID int, itemID int32) error {
	if g.onDelete != nil {
		hook := g.onDelete
		g.onDelete = nil
		hook()
	}
	g.ops = append(g.ops, "delete")
	delete(g.rows, stockKey{listID, itemID})
	return nil
}

// A sale that races a restock of the same product leaves the row the sale
// started: the restock's delete reaches the store before the sale's save,
// so a restart before the next restock restores the sold-down count rather
// than the full one.
func TestRestockDeleteCannotOvertakeNextSaleSave(t *testing.T) {
	t.Parallel()
	product := buylist.Product{BuyListID: 1, ItemID: 5900, Price: 1, RestockDelayMillis: int64(3 * time.Hour / time.Millisecond), MaxCount: 1}
	lists := buylist.NewTable([]buylist.List{{ID: 1, NPCID: 30001, Products: []buylist.Product{product}}})
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	store := &gapStore{rows: map[stockKey]StockRow{}}
	// A nil worker runs each write inline, so the store sees the writes
	// at the point the stock issues them.
	s := NewStock(lists, store, nil, func() time.Time { return now }, zerolog.Nop())

	if !s.Decrease(product, 1) {
		t.Fatal("first sale refused")
	}
	now = base.Add(3 * time.Hour)

	// The buyer acts in the window between the restock's memory change and
	// its delete, if that window is open: the lock is free there.
	store.onDelete = func() {
		if !s.mu.TryLock() {
			return // the delete runs under the lock: no buyer can act here
		}
		s.mu.Unlock()
		if !s.Decrease(product, 1) {
			t.Error("sale after the restock refused")
		}
	}
	s.Restock()
	if s.Count(product) == 1 {
		// No sale ran inside the restock: the buyer comes next.
		if !s.Decrease(product, 1) {
			t.Fatal("sale after the restock refused")
		}
	}

	want := []string{"save", "delete", "save"}
	if len(store.ops) != len(want) || store.ops[1] != "delete" || store.ops[2] != "save" {
		t.Fatalf("row writes = %v, want %v", store.ops, want)
	}
	row, ok := store.rows[stockKey{1, 5900}]
	if !ok {
		t.Fatal("buylists row missing while the product waits for its restock")
	}
	if row.Count != 0 || !row.NextRestock.Equal(now.Add(3*time.Hour)) {
		t.Fatalf("buylists row = %+v, want count 0 restocking at %v", row, now.Add(3*time.Hour))
	}
}
