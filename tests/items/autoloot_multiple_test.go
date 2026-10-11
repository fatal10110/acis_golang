package items

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: an auto-looted kill reward ends in Player.addItem(itemId,
// amount, true) (Monster.dropOrAutoLootItem, Monster.java:505-523), which
// names the whole amount as YOU_PICKED_UP_S2_S1 before adding it
// (Player.java:1830-1840). ItemContainer.addItem(itemId, count) (ItemContainer.java:
// 224-253) grows a held stackable without creating an instance, and for a
// non-stackable creates count count-1 instances, stopping after the first
// when MultipleItemDrop is off.

// autoLootRaidWeapons kills a grand boss whose one roll is three of the
// catalog's non-stackable weapon, with AutoLootRaid on, and returns the
// kill with the InventoryUpdate it left pending flushed: its weapon rows
// and the item-grant chat lines.
func autoLootRaidWeapons(t *testing.T, opts ...gameservertest.Option) (killedForDrops, []inventoryEntry, [][]byte) {
	t.Helper()
	k := killForDrops(t, killDropNpc("GrandBoss", [2]int32{killDropWeaponID, 3}), append(opts, gameservertest.WithAutoLootRaid(true))...)
	if ground := k.srv.GroundItems.Snapshots(nil); len(ground) != 0 {
		t.Fatalf("ground items = %+v, want none: AutoLootRaid loots a grand boss", ground)
	}
	var weapons []inventoryEntry
	for _, e := range inventoryUpdateAfterTick(t, k.srv, k.srv.Client) {
		if e.itemID == killDropWeaponID {
			weapons = append(weapons, e)
		}
	}
	return k, weapons, grantMessages(t, k.frames)
}

// TestAutoLootSplitsNonStackableUnderMultipleItemDrop: with the shipped
// MultipleItemDrop, three auto-looted weapons arrive as three count-1
// instances with their own object ids, shown added and persisted, named
// once for the whole amount.
func TestAutoLootSplitsNonStackableUnderMultipleItemDrop(t *testing.T) {
	t.Parallel()
	k, weapons, messages := autoLootRaidWeapons(t)

	held := k.srv.PlayerInventory(t, k.killer).ItemsByTemplateID(killDropWeaponID)
	if len(held) != 3 {
		t.Fatalf("held weapon instances = %d, want 3", len(held))
	}
	var ids []int32
	for _, inst := range held {
		if c := inst.CountValue(); c != 1 {
			t.Fatalf("weapon %d holds %d, want 1", inst.ObjectID, c)
		}
		ids = append(ids, inst.ObjectID)
	}
	if len(weapons) != 3 {
		t.Fatalf("InventoryUpdate weapon rows = %+v, want the three new instances", weapons)
	}
	var shown []int32
	for _, e := range weapons {
		if e.state != 1 || e.count != 1 {
			t.Fatalf("InventoryUpdate weapon row %+v, want added with count 1", e)
		}
		shown = append(shown, e.objID)
	}
	slices.Sort(ids)
	slices.Sort(shown)
	if !slices.Equal(ids, shown) || len(slices.Compact(slices.Clone(ids))) != 3 {
		t.Fatalf("InventoryUpdate object ids = %v, want the three distinct held ids %v", shown, ids)
	}

	if len(messages) != 1 {
		t.Fatalf("grant messages = %d, want one line for the whole amount", len(messages))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(killDropWeaponID), itemNumberParam(3))

	k.srv.FlushItems(t)
	rows := 0
	for _, inst := range persistedItems(t, k.srv, k.killer) {
		if inst.TemplateID == killDropWeaponID {
			rows++
		}
	}
	if rows != 3 {
		t.Fatalf("persisted weapon rows = %d, want 3", rows)
	}
}

// TestAutoLootKeepsOneInstanceWithoutMultipleItemDrop: with
// MultipleItemDrop off the same kill adds one weapon, still named for the
// whole amount rolled.
func TestAutoLootKeepsOneInstanceWithoutMultipleItemDrop(t *testing.T) {
	t.Parallel()
	k, weapons, messages := autoLootRaidWeapons(t, gameservertest.WithSingleItemDrop())

	if got := carriedCount(t, k.srv, k.killer, killDropWeaponID); got != 1 {
		t.Fatalf("carried weapons = %d, want 1", got)
	}
	if len(weapons) != 1 || weapons[0].state != 1 || weapons[0].count != 1 {
		t.Fatalf("InventoryUpdate weapon rows = %+v, want one added instance", weapons)
	}
	if len(messages) != 1 {
		t.Fatalf("grant messages = %d, want one line", len(messages))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(killDropWeaponID), itemNumberParam(3))
}

// TestAutoLootOntoHeldStackTakesNoObjectID: adena auto-looted onto the
// adena the killer holds grows that stack and keeps no object id, while a
// new stack keeps exactly one.
func TestAutoLootOntoHeldStackTakesNoObjectID(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		held    int32
		wantIDs int
	}{
		{name: "held stack", held: 5, wantIDs: 0},
		{name: "new stack", held: 0, wantIDs: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 1, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithAutoLoot(true),
			)
			c := srv.Client
			objID := srv.SoleObjectID(t)
			if tt.held > 0 {
				srv.GiveItem(t, objID, item.AdenaID, tt.held)
			}
			startInWorld(t, c)
			monster := srv.SpawnHostileNPCTemplateAt(t, killDropNpc("Monster", [2]int32{item.AdenaID, 100}), location.Location{X: spawnX + 50, Y: spawnY, Z: spawnZ})
			drainUntilQuiet(t, c)
			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("player missing from world state")
			}
			killer, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("player %T is not an online character", obj)
			}
			before := srv.HeldObjectIDs()
			if !monster.TakeDamage(1_000_000, killer) {
				t.Fatal("lethal hit did not kill the npc")
			}
			drainUntilQuiet(t, c)

			if got := carriedCount(t, srv, objID, item.AdenaID); got != int(tt.held)+100 {
				t.Fatalf("carried adena = %d, want %d", got, tt.held+100)
			}
			if got := srv.HeldObjectIDs() - before; got != tt.wantIDs {
				t.Fatalf("object ids kept by the kill = %d, want %d", got, tt.wantIDs)
			}
		})
	}
}
