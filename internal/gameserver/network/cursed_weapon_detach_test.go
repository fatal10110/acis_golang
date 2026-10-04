package network

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/cursedweapon"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// TestCursedWeaponEndBehindLogoutDeletesStoredWeapon: a weapon whose end
// the ticker reaches while its holder's logout is already queued finds the
// holder still in the world, so its release is posted to the holder's queue
// and runs after the logout. By then the logout has released the
// inventory's persistence and queued its last item save, so destroying the
// weapon in memory reaches no row. The release's own delete, queued on the
// holder's lane after that save, must still take the weapon out of the
// stored items, or the holder logs back in wielding an uncursed weapon the
// manager is free to drop again.
func TestCursedWeaponEndBehindLogoutDeletesStoredWeapon(t *testing.T) {
	const weaponID int32 = 30 // a weapon in testItemTemplates
	const weaponObjectID int32 = 6100
	ctx := context.Background()

	link, _, _, _, holder, _ := newDirectTradeFixture(t)
	db := sqltest.SharedDB(t)
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() {
		if err := worker.Close(context.Background()); err != nil {
			t.Errorf("close persistence worker: %v", err)
		}
	})
	link.persist = worker
	instances := task.NewItemInstances(gamesql.NewItemFlushStore(db), link.itemTemplates, worker, link.itemWrites, zerolog.Nop())
	link.itemInstances = instances

	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{
		ItemID: weaponID, Name: "Cursed", DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10,
	}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	link.cursed = cursedweapon.New(table, gamesql.NewCursedWeaponStore(db), worker, zerolog.Nop(), nil)

	// The holder's queue runs only when the test drives it, so the order
	// of the logout and the release is the order they were posted in.
	loop := sim.NewInline(time.Unix(0, 0))
	holder.Character.Live.SetQueue(loop.NewQueue("holder"))
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	id := holder.ObjectID()
	inv := itemcontainer.NewPlayerInventoryWithDelivery(id, link.itemTemplates, nil, link.itemPersister(id))
	holder.Character.AttachRuntime(tmpl, inv)
	inv.AddNew(weaponID, 1, weaponObjectID)
	if _, ok := link.cursed.Activate(weaponID, cursedweapon.Holder{ObjectID: id, Karma: 100, PKKills: 3}, func(int) int { return 0 }); !ok {
		t.Fatal("Activate refused the weapon")
	}
	holder.SetCursedWeapon(weaponID, 1)
	if err := instances.Save(ctx); err != nil {
		t.Fatalf("save items: %v", err)
	}
	if err := worker.Flush(ctx); err != nil {
		t.Fatalf("flush lanes: %v", err)
	}
	if n := countStoredItem(t, db, id, weaponID); n != 1 {
		t.Fatalf("stored weapons before the logout = %d, want 1", n)
	}

	// The logout is queued first, as the connection goroutine posts it;
	// the ticker then ends the weapon while the holder is still listed.
	if !holder.Queue().Post(func() { link.detachLivePlayer(holder) }) {
		t.Fatal("post the logout: queue closed")
	}
	end, ok := link.cursed.Expire(weaponID)
	if !ok {
		t.Fatal("Expire found the weapon not out")
	}
	link.endCursedWeapon(end, nil)
	if _, online := link.livePlayerByID(id); !online {
		t.Fatal("holder left the world before its queue ran: the release was not posted behind the logout")
	}
	loop.Run()
	if _, online := link.livePlayerByID(id); online {
		t.Fatal("holder still in the world after its logout ran")
	}
	if err := instances.Save(ctx); err != nil {
		t.Fatalf("save items: %v", err)
	}
	if err := worker.Flush(ctx); err != nil {
		t.Fatalf("flush lanes: %v", err)
	}

	if n := countStoredItem(t, db, id, weaponID); n != 0 {
		t.Fatalf("stored weapons after the end = %d, want 0", n)
	}
	if inv.ItemByTemplateID(weaponID) != nil || holder.CursedWeaponEquipped() {
		t.Fatal("the release left the weapon with its holder")
	}
}

func countStoredItem(t *testing.T, db *sql.DB, ownerID, itemID int32) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id = ?", ownerID, itemID).Scan(&n); err != nil {
		t.Fatalf("count stored items: %v", err)
	}
	return n
}
