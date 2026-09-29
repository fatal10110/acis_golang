package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// autoLootWeaponID is the catalog's non-stackable weapon: auto-looting it
// always needs a new slot.
const autoLootWeaponID int32 = 30

// autoLootAdena is the guaranteed adena drop of autoLootMonsterTemplate.
const autoLootAdena = 10

// autoLootMonsterTemplate is a monster with one guaranteed adena drop and
// one guaranteed weapon drop.
func autoLootMonsterTemplate() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Drops: []item.DropCategory{
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: autoLootAdena, Max: autoLootAdena, Chance: 100}}},
			{Kind: item.DropNormal, Chance: 100, Drops: []item.Drop{{ItemID: autoLootWeaponID, Min: 1, Max: 1, Chance: 100}}},
		},
	}
}

// killAutoLootMonster boots a player holding one weapon and some adena, with
// auto-loot on and noDwarfSlots inventory slots, and kills the drop monster
// with a hit from that player.
func killAutoLootMonster(t *testing.T, noDwarfSlots int) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithInventorySlots(noDwarfSlots, noDwarfSlots),
		gameservertest.WithAutoLoot(true),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, autoLootWeaponID, 1)
	srv.GiveItem(t, objID, item.AdenaID, 5)
	startInWorld(t, c)

	monster := srv.SpawnHostileNPCTemplateAt(t, autoLootMonsterTemplate(), location.Location{X: spawnX + 50, Y: spawnY, Z: spawnZ})
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	killer, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if !monster.TakeDamage(1_000_000, killer) {
		t.Fatal("lethal hit did not kill the monster")
	}
	drainUntilQuiet(t, c)
	return srv, objID
}

// TestAutoLootAtInventoryLimitDropsNewStack pins Monster.dropOrAutoLootItem's
// validateCapacityByItemId gate: with auto-loot on and the inventory at its
// slot limit, adena still merges into the held stack, while the weapon,
// which needs a new slot, falls to the ground reserved to the killer and the
// inventory stays at the limit.
func TestAutoLootAtInventoryLimitDropsNewStack(t *testing.T) {
	t.Parallel()
	srv, objID := killAutoLootMonster(t, 2)

	if got := carriedCount(t, srv, objID, item.AdenaID); got != 5+autoLootAdena {
		t.Fatalf("carried adena = %d, want %d: a held stackable still merges at the limit", got, 5+autoLootAdena)
	}
	if got := carriedCount(t, srv, objID, autoLootWeaponID); got != 1 {
		t.Fatalf("carried weapons = %d, want 1: the looted weapon needs a slot the inventory lacks", got)
	}
	if got := srv.PlayerInventory(t, objID).Size(); got != 2 {
		t.Fatalf("inventory size = %d, want the limit 2", got)
	}
	drops := srv.GroundItems.Snapshots(nil)
	if len(drops) != 1 || drops[0].TemplateID != autoLootWeaponID || drops[0].Count != 1 {
		t.Fatalf("ground drops = %+v, want the one weapon that did not fit", drops)
	}
	if drops[0].OwnerID != objID {
		t.Fatalf("dropped weapon reserved to %d, want the killer %d", drops[0].OwnerID, objID)
	}
}

// TestAutoLootBelowInventoryLimitTakesNewStack is the control: one free slot
// takes the looted weapon straight into the inventory and nothing drops.
func TestAutoLootBelowInventoryLimitTakesNewStack(t *testing.T) {
	t.Parallel()
	srv, objID := killAutoLootMonster(t, 3)

	if got := carriedCount(t, srv, objID, autoLootWeaponID); got != 2 {
		t.Fatalf("carried weapons = %d, want 2: the looted weapon fits the free slot", got)
	}
	if got := carriedCount(t, srv, objID, item.AdenaID); got != 5+autoLootAdena {
		t.Fatalf("carried adena = %d, want %d", got, 5+autoLootAdena)
	}
	if drops := srv.GroundItems.Snapshots(nil); len(drops) != 0 {
		t.Fatalf("ground drops = %+v, want none: everything was auto-looted", drops)
	}
}
