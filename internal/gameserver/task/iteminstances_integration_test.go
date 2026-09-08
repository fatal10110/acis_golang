// Package task_test is an external test package (not task) because
// gamesql transitively imports task (via model/actor/player); only a
// separate test package can import both without an import cycle.
package task_test

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// TestItemInstancesSaveChunksIndependently pins the #2295 fix: a Save batch
// spanning more than one ItemInstanceSaveChunkSize commits its chunks as
// separate transactions, so one chunk's failure does not undo, or retry,
// another chunk's success. Object id 1 sits in the first chunk (chunks split
// on sorted ids) and carries an augmentation; dropping the augmentations
// table makes only that chunk's transaction fail, while the second chunk —
// plain items with no augmentation — commits on its own.
func TestItemInstancesSaveChunksIndependently(t *testing.T) {
	ctx := context.Background()
	// Uses its own container, not SharedDB: this test drops a table out
	// from under the schema, which would corrupt every other test sharing
	// the package's container.
	db := sqltest.NewDB(t)
	flusher := gamesql.NewItemFlushStore(db)
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}}})
	instances := task.NewItemInstances(flusher, templates)

	const total = task.ItemInstanceSaveChunkSize + 50
	items := make([]*item.Instance, 0, total)
	for id := int32(1); id <= total; id++ {
		inst := &item.Instance{ObjectID: id, TemplateID: 999, OwnerID: 1, Count: 1, Location: item.LocationInventory, ManaLeft: -1}
		if id == 1 {
			inst.TemplateID = 10
			inst.Augmentation = &item.Augmentation{Attributes: 1, SkillID: 1, SkillLevel: 1}
		}
		items = append(items, inst)
		instances.Add(inst)
	}

	if _, err := db.ExecContext(ctx, "DROP TABLE augmentations"); err != nil {
		t.Fatalf("drop augmentations table: %v", err)
	}

	if err := instances.Save(ctx); err == nil {
		t.Fatal("Save() error = nil, want the first chunk's augmentation write to fail")
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items").Scan(&count); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count != 50 {
		t.Fatalf("persisted item rows = %d, want 50 (only the second chunk committed)", count)
	}

	if !instances.Contains(items[1]) {
		t.Fatal("an item from the failed first chunk should stay pending for retry")
	}
	if instances.Contains(items[total-1]) {
		t.Fatal("an item from the successful second chunk should have been cleared from pending")
	}
}
