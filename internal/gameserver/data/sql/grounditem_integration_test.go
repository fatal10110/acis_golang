//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestGroundItemStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewGroundItemStore(sqltest.NewDB(t))

	initial, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("initial Load() error = %v", err)
	}
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

	reloaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("reloaded Load() error = %v", err)
	}
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

	// Load must not clear the table: a second Load before Clear sees the
	// same rows, matching the two-phase restore (#432) callers rely on to
	// avoid deleting persisted state before every row hydrates.
	reloadedAgain, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if len(reloadedAgain) != 2 {
		t.Fatalf("second Load() len = %d, want 2 (Load must not clear)", len(reloadedAgain))
	}

	if err := store.Clear(ctx); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	emptyAfterClear, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("empty Load() error = %v", err)
	}
	if len(emptyAfterClear) != 0 {
		t.Fatalf("Load() after Clear = %v, want empty", emptyAfterClear)
	}

	rows[0].Count = 777
	rows[0].X = 99
	if err := store.Save(ctx, rows[:1]); err != nil {
		t.Fatalf("update Save() error = %v", err)
	}
	updated, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("updated Load() error = %v", err)
	}
	if len(updated) != 1 || updated[0].ObjectID != rows[0].ObjectID || updated[0].Count != 777 || updated[0].X != 99 {
		t.Fatalf("updated rows = %+v", updated)
	}

	if err := store.Save(ctx, nil); err != nil {
		t.Fatalf("empty Save() error = %v", err)
	}
	missing, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("missing Load() error = %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing rows = %v, want empty", missing)
	}
}
