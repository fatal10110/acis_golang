//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestGroundItemsBootWiringRoundTrip drives provideGroundItems and the
// shutdown hook startGroundItems installs — the exact sequence
// newGameServerApp's fx graph runs — against a real database, proving the
// boot-time restore and shutdown-time save wired into cmd/gameserver
// actually round-trip instead of only type-checking.
func TestGroundItemsBootWiringRoundTrip(t *testing.T) {
	db := sqltest.NewDB(t)
	ctx := context.Background()

	tmpl := &item.Template{ID: 57, Kind: item.KindEtcItem, Duration: -1}
	data := &gameData{Items: item.NewTable([]*item.Template{tmpl})}
	opts := task.DefaultGroundItemOptions()
	log := zerolog.Nop()

	// Seed one persisted row, as if a previous server session had saved it
	// at shutdown.
	seedStore := gamesql.NewGroundItemStore(db)
	if err := seedStore.Save(ctx, []item.GroundSnapshot{
		{Instance: item.Instance{ObjectID: 0x10000101, TemplateID: 57, Count: 5}, X: 10, Y: 20, Z: 30},
	}); err != nil {
		t.Fatalf("seed Save() error = %v", err)
	}

	state := world.New()
	items, store, err := provideGroundItems(db, state, opts, data, log)
	if err != nil {
		t.Fatalf("provideGroundItems() error = %v", err)
	}
	if got := items.Len(); got != 1 {
		t.Fatalf("restored items.Len() = %d, want 1", got)
	}
	if _, ok := state.Object(0x10000101); !ok {
		t.Fatal("restored item not spawned into world state")
	}

	// The row must be cleared only after hydrating: a fresh Load right
	// after boot must come back empty rather than double-restoring it.
	reloadedRows, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() after boot error = %v", err)
	}
	if len(reloadedRows) != 0 {
		t.Fatalf("Load() after boot = %v, want empty (row must be cleared once hydrated)", reloadedRows)
	}

	// Drop a second, live item, then run the exact shutdown hook
	// startGroundItems installs (store.Save(ctx, items.Snapshots(nil))).
	ground, err := grounditem.New(item.Instance{ObjectID: 0x10000102, TemplateID: 57, Count: 9}, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New() error = %v", err)
	}
	items.Drop(ground, task.DropOptions{X: 40, Y: 50, Z: 60})

	if err := store.Save(ctx, items.Snapshots(nil)); err != nil {
		t.Fatalf("shutdown Save() error = %v", err)
	}

	// A fresh boot must now restore both: the item carried over from the
	// first seed (still on the ground, never picked up) and the item
	// dropped live during this session.
	nextState := world.New()
	nextItems, _, err := provideGroundItems(db, nextState, opts, data, log)
	if err != nil {
		t.Fatalf("second provideGroundItems() error = %v", err)
	}
	if got := nextItems.Len(); got != 2 {
		t.Fatalf("second boot restored items.Len() = %d, want 2", got)
	}
	if _, ok := nextState.Object(0x10000101); !ok {
		t.Fatal("second boot did not restore the carried-over item")
	}
	if _, ok := nextState.Object(0x10000102); !ok {
		t.Fatal("second boot did not restore the live-dropped item")
	}
}
