package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestCursedWeaponGroundItemExcludedFromShutdownPersistence mirrors Java's
// ItemsOnGroundTaskManager.save(), which skips cursed weapons before writing
// items_on_ground to prevent a duplicate save: a cursed-weapon ground item
// must never appear in a persisted snapshot, while an ordinary ground item
// dropped alongside it still does.
func TestCursedWeaponGroundItemExcludedFromShutdownPersistence(t *testing.T) {
	const cursedItemID = 8190 // Demonic Sword Zariche

	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{ItemID: cursedItemID}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable() error: %v", err)
	}

	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCursedWeapons(table),
	)

	objID := srv.SoleObjectID(t)
	srv.SeedGroundItem(t, objID, cursedItemID, 1, spawnX, spawnY, spawnZ)
	srv.SeedGroundItem(t, objID, item.AdenaID, 100, spawnX, spawnY, spawnZ)

	srv.FlushGroundItems(t)

	rows, err := groundRows(t, srv)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TemplateID != item.AdenaID || rows[0].Count != 100 {
		t.Fatalf("items_on_ground rows = %+v, want one adena row count 100", rows)
	}
}
