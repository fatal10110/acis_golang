//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// loadAndClear mirrors how a real boot sequence must use the split API:
// hydrate every row in memory before clearing the table.
func loadAndClear(t *testing.T, ctx context.Context, store *GroundItemStore) []item.GroundSnapshot {
	t.Helper()
	rows, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := store.Clear(ctx); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	return rows
}

func TestGroundItemStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewGroundItemStore(sqltest.NewDB(t))

	initial := loadAndClear(t, ctx, store)
	if len(initial) != 0 {
		t.Fatalf("initial Load() = %v, want empty", initial)
	}

	rows := []item.GroundSnapshot{
		{
			Instance:       item.Instance{ObjectID: 0x10000101, TemplateID: 57, Count: 500},
			X:              10,
			Y:              20,
			Z:              -30,
			TimeLeftMillis: 0,
		},
		{
			Instance:       item.Instance{ObjectID: 0x10000102, TemplateID: 10, Count: 1, EnchantLevel: 7},
			X:              40,
			Y:              50,
			Z:              -60,
			TimeLeftMillis: 12_345,
		},
	}
	if err := store.Save(ctx, rows); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded := loadAndClear(t, ctx, NewGroundItemStore(store.db))
	if len(reloaded) != 2 {
		t.Fatalf("reloaded len = %d, want 2", len(reloaded))
	}
	byID := map[int32]item.GroundSnapshot{}
	for _, row := range reloaded {
		byID[row.ObjectID] = row
	}
	if got := byID[0x10000101]; got.TemplateID != 57 || got.Count != 500 || got.X != 10 || got.TimeLeftMillis != 0 {
		t.Fatalf("adena row = %+v", got)
	}
	if got := byID[0x10000102]; got.TemplateID != 10 || got.EnchantLevel != 7 || got.Z != -60 || got.TimeLeftMillis != 12_345 {
		t.Fatalf("weapon row = %+v", got)
	}

	emptyAfterLoad := loadAndClear(t, ctx, NewGroundItemStore(store.db))
	if len(emptyAfterLoad) != 0 {
		t.Fatalf("Load() after restart clear = %v, want empty", emptyAfterLoad)
	}

	rows[0].Count = 777
	rows[0].X = 99
	if err := store.Save(ctx, rows[:1]); err != nil {
		t.Fatalf("update Save() error = %v", err)
	}
	updated := loadAndClear(t, ctx, NewGroundItemStore(store.db))
	if len(updated) != 1 || updated[0].ObjectID != rows[0].ObjectID || updated[0].Count != 777 || updated[0].X != 99 {
		t.Fatalf("updated rows = %+v", updated)
	}

	if err := store.Save(ctx, nil); err != nil {
		t.Fatalf("empty Save() error = %v", err)
	}
	missing := loadAndClear(t, ctx, NewGroundItemStore(store.db))
	if len(missing) != 0 {
		t.Fatalf("missing rows = %v, want empty", missing)
	}
}

// TestGroundItemStoreLoadWithoutClearPreservesRows is the regression test
// for the restore-data-loss bug: if a caller reads rows with Load but never
// calls Clear (e.g. because hydration failed), the persisted rows must
// still be there on the next Load.
func TestGroundItemStoreLoadWithoutClearPreservesRows(t *testing.T) {
	ctx := context.Background()
	store := NewGroundItemStore(sqltest.NewDB(t))

	rows := []item.GroundSnapshot{
		{Instance: item.Instance{ObjectID: 0x10000101, TemplateID: 57, Count: 500}, X: 10, Y: 20, Z: -30},
	}
	if err := store.Save(ctx, rows); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	first, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first Load() len = %d, want 1", len(first))
	}
	// Simulate a hydration failure: Clear is deliberately not called.

	second, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if len(second) != 1 || second[0].ObjectID != 0x10000101 {
		t.Fatalf("second Load() = %+v, want the row to still be persisted", second)
	}
}
