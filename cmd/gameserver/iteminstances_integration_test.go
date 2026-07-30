//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"go.uber.org/fx/fxtest"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

const persistTestTemplateID int32 = 57

func itemPersistenceTestData() *gameData {
	return &gameData{Items: item.NewTable([]*item.Template{
		{ID: persistTestTemplateID, Name: "Adena", Kind: item.KindEtcItem, Stackable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
	})}
}

// TestItemInstancesBootWiringPersistsServerSideMutation drives the wiring
// provideItemInstances installs: an inventory item mutated with no client
// request involved must reach the items table on the task's own tick.
func TestItemInstancesBootWiringPersistsServerSideMutation(t *testing.T) {
	db := sqltest.NewDB(t)
	ctx := context.Background()

	data := itemPersistenceTestData()
	items := provideItemInstances(db, gamesql.NewItemStore(db), data)

	inv := itemcontainer.NewPlayerInventory(0x10000001, data.Items)
	inv.SetItemPersister(items.Add)

	// A server-side grant (a kill reward's auto-loot, say): no packet is
	// decoded, nothing calls a store directly.
	inst := inv.AddNew(persistTestTemplateID, 100, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	inst.AddCount(50)

	if !items.Contains(inst) {
		t.Fatal("mutated item is not pending persistence")
	}

	// One tick: exactly what the ticker startItemInstances launches runs.
	if err := items.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var count, ownerID int
	var loc string
	row := db.QueryRowContext(ctx, "SELECT count, owner_id, loc FROM items WHERE object_id = ?", 0x20000001)
	if err := row.Scan(&count, &ownerID, &loc); err != nil {
		t.Fatalf("read persisted item: %v", err)
	}
	if count != 150 {
		t.Errorf("persisted count = %d, want 150", count)
	}
	if ownerID != 0x10000001 {
		t.Errorf("persisted owner_id = %d, want %d", ownerID, 0x10000001)
	}
	if loc != item.LocationInventory.String() {
		t.Errorf("persisted loc = %q, want %q", loc, item.LocationInventory.String())
	}

	// The pending set is released once written, so an unchanged item isn't
	// rewritten on every later tick.
	if items.Contains(inst) {
		t.Error("item still pending after Save()")
	}
}

// TestItemInstancesShutdownFlushesPending is the restart-equivalent case:
// an item mutated between ticks must be written by the shutdown hook rather
// than lost with the process.
func TestItemInstancesShutdownFlushesPending(t *testing.T) {
	db := sqltest.NewDB(t)
	ctx := context.Background()

	data := itemPersistenceTestData()
	items := provideItemInstances(db, gamesql.NewItemStore(db), data)

	lc := fxtest.NewLifecycle(t)
	startItemInstances(lc, items, zerolog.Nop())
	lc.RequireStart()

	inv := itemcontainer.NewPlayerInventory(0x10000002, data.Items)
	inv.SetItemPersister(items.Add)
	inst := inv.AddNew(persistTestTemplateID, 42, 0x20000002)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}

	// No tick elapses (the cadence is a minute); shutdown must still save.
	lc.RequireStop()

	var count int
	row := db.QueryRowContext(ctx, "SELECT count FROM items WHERE object_id = ?", 0x20000002)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("read persisted item after shutdown: %v", err)
	}
	if count != 42 {
		t.Errorf("persisted count after shutdown = %d, want 42", count)
	}
}

// TestItemInstancesPersistsDestruction proves a consumed item's row is
// deleted rather than left behind as a phantom stack after a restart.
func TestItemInstancesPersistsDestruction(t *testing.T) {
	db := sqltest.NewDB(t)
	ctx := context.Background()

	data := itemPersistenceTestData()
	items := provideItemInstances(db, gamesql.NewItemStore(db), data)

	inv := itemcontainer.NewPlayerInventory(0x10000003, data.Items)
	inv.SetItemPersister(items.Add)
	inst := inv.AddNew(persistTestTemplateID, 10, 0x20000003)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	if err := items.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// A consumed stack: destroyed server-side, not through a client packet.
	if got := inv.DestroyItem(inst, 10); got == nil {
		t.Fatal("DestroyItem() = nil")
	}
	if err := items.Save(ctx); err != nil {
		t.Fatalf("Save() after destroy error = %v", err)
	}

	var found int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE object_id = ?", 0x20000003)
	if err := row.Scan(&found); err != nil {
		t.Fatalf("count rows after destroy: %v", err)
	}
	if found != 0 {
		t.Errorf("rows for destroyed item = %d, want 0", found)
	}
}
