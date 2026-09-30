package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: a ground pickup ends in Player.addAdena(count, true) for adena
// merged into a held adena stack, otherwise Player.addItem(ItemInstance,
// true) (PlayerAI.thinkPickUp, PlayerAI.java:394-403); an auto-looted kill
// reward ends in addAdena(amount, true) or addItem(itemId, amount, true)
// (Monster.dropOrAutoLootItem, Monster.java:505-517).
//
// addAdena sends EARNED_S1_ADENA (52) with the amount as a number (type 1)
// (Player.java:1694-1700). addItem(ItemInstance) sends YOU_PICKED_UP_S2_S1
// (29) with the item name and the count as a number (type 1) for a stack
// of more than one, YOU_PICKED_UP_A_S1_S2 (369) with the enchant level then
// the item name for a single enchanted item, and YOU_PICKED_UP_S1 (30)
// otherwise (Player.java:1782-1790). addItem(itemId, count) sends the same
// 29 with the count as an item number (type 6), or 30 (Player.java:
// 1830-1836). Both addItem forms then put a bow's matching arrows on when
// the added item is an arrow, the attack type is BOW and the left hand is
// empty (Player.java:1801-1802, 1848-1849; checkAndEquipArrows :3452-3467).

// grantParam is one decoded SystemMessage parameter.
type grantParam struct{ kind, value int32 }

// decodeGrantMessage decodes a SystemMessage whose parameters are all
// single-int32 types (number, item name, item number).
func decodeGrantMessage(t *testing.T, frame []byte) (int32, []grantParam) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	params := make([]grantParam, r.ReadInt32())
	for i := range params {
		params[i].kind = r.ReadInt32()
		params[i].value = r.ReadInt32()
	}
	return id, params
}

// grantMessageIDs are the self chat lines an item grant can send.
var grantMessageIDs = map[int32]bool{
	serverpackets.SystemMessageEarnedS1Adena:    true,
	serverpackets.SystemMessageYouPickedUpS2S1:  true,
	serverpackets.SystemMessageYouPickedUpS1:    true,
	serverpackets.SystemMessageYouPickedUpAS1S2: true,
}

// grantMessages returns the item-grant SystemMessage frames among frames.
func grantMessages(t *testing.T, frames [][]byte) [][]byte {
	t.Helper()
	var out [][]byte
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id, _ := decodeGrantMessage(t, f); grantMessageIDs[id] {
			out = append(out, f)
		}
	}
	return out
}

// assertGrantMessage requires frame to be message id with exactly params.
func assertGrantMessage(t *testing.T, frame []byte, id int32, params ...grantParam) {
	t.Helper()
	gotID, got := decodeGrantMessage(t, frame)
	if gotID != id {
		t.Fatalf("system message id = %d, want %d", gotID, id)
	}
	if len(got) != len(params) {
		t.Fatalf("message %d params = %+v, want %+v", id, got, params)
	}
	for i := range params {
		if got[i] != params[i] {
			t.Fatalf("message %d params = %+v, want %+v", id, got, params)
		}
	}
}

// numberParam is a plain number parameter (type 1).
func numberParam(v int32) grantParam {
	return grantParam{serverpackets.SystemMessageParamNumber, v}
}

// itemNameParam is an item-name parameter (type 3).
func itemNameParam(v int32) grantParam {
	return grantParam{serverpackets.SystemMessageParamItemName, v}
}

// itemNumberParam is an item-number parameter (type 6).
func itemNumberParam(v int32) grantParam {
	return grantParam{serverpackets.SystemMessageParamItemNumber, v}
}

// pickUpSoleGroundItem clicks the one ground item and returns every frame
// the pickup sends ahead of the InventoryUpdate tick, which it leaves
// pending.
func pickUpSoleGroundItem(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	groundID := soleGroundObjectID(t, srv)
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	frames := collectUntilQuiet(t, c)
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeInventoryUpdate {
			t.Fatal("InventoryUpdate reached the client before its tick")
		}
	}
	// The chat line follows the ground item's removal.
	deleted := false
	for _, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeDeleteObject:
			deleted = true
		case len(grantMessages(t, [][]byte{f})) == 1 && !deleted:
			t.Fatal("pickup chat line sent before the ground item's DeleteObject")
		}
	}
	return frames
}

// bootPicker boots a player in the world holding the given items (template
// id to count) and returns the server with the player's object id.
func bootPicker(t *testing.T, held map[int32]int32) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	for templateID, count := range held {
		srv.GiveItem(t, objID, templateID, count)
	}
	startInWorld(t, srv.Client)
	return srv, objID
}

// TestGroundPickupNamesTheItem pins the picker's own chat line for each
// pickup shape.
func TestGroundPickupNamesTheItem(t *testing.T) {
	t.Parallel()
	const potionID int32 = 1060

	t.Run("adena merged into held adena", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootPicker(t, map[int32]int32{item.AdenaID: 100})
		srv.SeedGroundItem(t, objID, item.AdenaID, 40, spawnX, spawnY, spawnZ)
		drainUntilQuiet(t, srv.Client)

		msgs := grantMessages(t, pickUpSoleGroundItem(t, srv, srv.Client))
		if len(msgs) != 1 {
			t.Fatalf("grant messages = %d, want 1", len(msgs))
		}
		assertGrantMessage(t, msgs[0], serverpackets.SystemMessageEarnedS1Adena, numberParam(40))
		if got := carriedCount(t, srv, objID, item.AdenaID); got != 140 {
			t.Fatalf("carried adena = %d, want 140", got)
		}
	})

	t.Run("first adena is a picked-up stack", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootPicker(t, nil)
		srv.SeedGroundItem(t, objID, item.AdenaID, 40, spawnX, spawnY, spawnZ)
		drainUntilQuiet(t, srv.Client)

		msgs := grantMessages(t, pickUpSoleGroundItem(t, srv, srv.Client))
		if len(msgs) != 1 {
			t.Fatalf("grant messages = %d, want 1", len(msgs))
		}
		assertGrantMessage(t, msgs[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(item.AdenaID), numberParam(40))
	})

	t.Run("potion stack", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootPicker(t, nil)
		srv.SeedGroundItem(t, objID, potionID, 5, spawnX, spawnY, spawnZ)
		drainUntilQuiet(t, srv.Client)

		msgs := grantMessages(t, pickUpSoleGroundItem(t, srv, srv.Client))
		if len(msgs) != 1 {
			t.Fatalf("grant messages = %d, want 1", len(msgs))
		}
		assertGrantMessage(t, msgs[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(potionID), numberParam(5))
	})

	t.Run("single unenchanted item", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootPicker(t, nil)
		srv.SeedGroundItem(t, objID, autoLootWeaponID, 1, spawnX, spawnY, spawnZ)
		drainUntilQuiet(t, srv.Client)

		msgs := grantMessages(t, pickUpSoleGroundItem(t, srv, srv.Client))
		if len(msgs) != 1 {
			t.Fatalf("grant messages = %d, want 1", len(msgs))
		}
		assertGrantMessage(t, msgs[0], serverpackets.SystemMessageYouPickedUpS1, itemNameParam(autoLootWeaponID))
	})

	t.Run("single enchanted item", func(t *testing.T) {
		t.Parallel()
		srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
		c := srv.Client
		objID := srv.SoleObjectID(t)
		weapon := srv.GiveItem(t, objID, autoLootWeaponID, 1)
		inst := mustFindItem(t, srv, objID, weapon)
		inst.EnchantLevel = 3
		if err := srv.Items.Update(context.Background(), inst); err != nil {
			t.Fatalf("seed enchant level: %v", err)
		}
		startInWorld(t, c)
		c.Send(encodeRequestDropItem(weapon, 1, spawnX, spawnY, spawnZ))
		drainUntilQuiet(t, c)
		srv.InventoryUpdates.Tick()
		drainUntilQuiet(t, c)

		msgs := grantMessages(t, pickUpSoleGroundItem(t, srv, c))
		if len(msgs) != 1 {
			t.Fatalf("grant messages = %d, want 1", len(msgs))
		}
		assertGrantMessage(t, msgs[0], serverpackets.SystemMessageYouPickedUpAS1S2, numberParam(3), itemNameParam(autoLootWeaponID))
	})
}

// TestGroundPickupEquipsArrowsForWornBow: a bow user with an empty left
// hand wears picked-up arrows of the bow's grade at once.
func TestGroundPickupEquipsArrowsForWornBow(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	bow := srv.GiveItem(t, objID, fixtureBowID, 1)
	startInWorld(t, c)
	c.Send(encodeUseItem(bow, false))
	drainUntilQuiet(t, c)
	inventoryUpdateAfterTick(t, srv, c)
	drainUntilQuiet(t, c)

	srv.SeedGroundItem(t, objID, fixtureArrowID, 20, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	msgs := grantMessages(t, pickUpSoleGroundItem(t, srv, c))
	if len(msgs) != 1 {
		t.Fatalf("grant messages = %d, want 1", len(msgs))
	}
	assertGrantMessage(t, msgs[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(fixtureArrowID), numberParam(20))

	arrows := wornLeftHand(t, srv, objID)
	if arrows == nil || arrows.TemplateID != fixtureArrowID {
		t.Fatalf("left hand = %+v, want the picked-up arrows", arrows)
	}
	update := inventoryUpdateAfterTick(t, srv, c)
	if e, ok := update[arrows.ObjectID]; !ok || e.equipped != 1 {
		t.Fatalf("arrow InventoryUpdate entry = %+v (present %v), want equipped", e, ok)
	}
	assertWorn(t, srv, objID, arrows.ObjectID, itemcontainer.LHand)
}

// TestGroundPickupLeavesArrowsWithoutBow is the control: without a bow the
// picked-up arrows stay in the bag.
func TestGroundPickupLeavesArrowsWithoutBow(t *testing.T) {
	t.Parallel()
	srv, objID := bootPicker(t, nil)
	srv.SeedGroundItem(t, objID, fixtureArrowID, 20, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, srv.Client)
	pickUpSoleGroundItem(t, srv, srv.Client)
	if got := wornLeftHand(t, srv, objID); got != nil {
		t.Fatalf("left hand = %+v, want empty without a bow", got)
	}
}

// wornLeftHand returns the live left-hand item of the player, or nil.
func wornLeftHand(t *testing.T, srv *gameservertest.Server, objID int32) *item.Instance {
	t.Helper()
	return srv.PlayerInventory(t, objID).ItemAt(itemcontainer.LHand)
}

// grantMonsterTemplate is a monster whose every listed drop is guaranteed.
func grantMonsterTemplate(drops ...item.Drop) *npc.Template {
	tmpl := &npc.Template{
		ID: 101, TemplateID: 101, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	for _, d := range drops {
		kind := item.DropNormal
		if d.ItemID == item.AdenaID {
			kind = item.DropCurrency
		}
		tmpl.Drops = append(tmpl.Drops, item.DropCategory{Kind: kind, Chance: 100, Drops: []item.Drop{d}})
	}
	return tmpl
}

// killForAutoLoot spawns tmpl next to the player, kills it with the
// player's hit and returns every frame the kill sent the killer.
func killForAutoLoot(t *testing.T, srv *gameservertest.Server, objID int32, tmpl *npc.Template) [][]byte {
	t.Helper()
	c := srv.Client
	monster := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: spawnX + 50, Y: spawnY, Z: spawnZ})
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
	return collectUntilQuiet(t, c)
}

// grantMessagesByID indexes grant messages by id; auto-loot delivers its
// rolled stacks in no fixed order.
func grantMessagesByID(t *testing.T, frames [][]byte) map[int32][][]byte {
	t.Helper()
	out := map[int32][][]byte{}
	for _, f := range grantMessages(t, frames) {
		id, _ := decodeGrantMessage(t, f)
		out[id] = append(out[id], f)
	}
	return out
}

// TestAutoLootNamesEachStack: auto-looted adena names its amount, a stack
// names its item and count as an item number, a single item names itself.
func TestAutoLootNamesEachStack(t *testing.T) {
	t.Parallel()
	const potionID int32 = 1060
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAutoLoot(true),
	)
	objID := srv.SoleObjectID(t)
	startInWorld(t, srv.Client)

	frames := killForAutoLoot(t, srv, objID, grantMonsterTemplate(
		item.Drop{ItemID: item.AdenaID, Min: 10, Max: 10, Chance: 100},
		item.Drop{ItemID: potionID, Min: 3, Max: 3, Chance: 100},
		item.Drop{ItemID: autoLootWeaponID, Min: 1, Max: 1, Chance: 100},
	))
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeInventoryUpdate {
			t.Fatal("InventoryUpdate reached the client before its tick")
		}
	}
	byID := grantMessagesByID(t, frames)
	if len(byID[serverpackets.SystemMessageEarnedS1Adena]) != 1 || len(byID[serverpackets.SystemMessageYouPickedUpS2S1]) != 1 || len(byID[serverpackets.SystemMessageYouPickedUpS1]) != 1 || len(grantMessages(t, frames)) != 3 {
		t.Fatalf("grant messages by id = %v, want one adena, one stack and one single-item line", byID)
	}
	assertGrantMessage(t, byID[serverpackets.SystemMessageEarnedS1Adena][0], serverpackets.SystemMessageEarnedS1Adena, numberParam(10))
	assertGrantMessage(t, byID[serverpackets.SystemMessageYouPickedUpS2S1][0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(potionID), itemNumberParam(3))
	assertGrantMessage(t, byID[serverpackets.SystemMessageYouPickedUpS1][0], serverpackets.SystemMessageYouPickedUpS1, itemNameParam(autoLootWeaponID))
}

// TestAutoLootDroppedStackIsSilent: an item that does not fit falls to the
// ground and names nothing, while adena merged into the held stack still
// names its amount.
func TestAutoLootDroppedStackIsSilent(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithInventorySlots(2, 2),
		gameservertest.WithAutoLoot(true),
	)
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, autoLootWeaponID, 1)
	srv.GiveItem(t, objID, item.AdenaID, 5)
	startInWorld(t, srv.Client)

	frames := killForAutoLoot(t, srv, objID, autoLootMonsterTemplate())
	msgs := grantMessages(t, frames)
	if len(msgs) != 1 {
		t.Fatalf("grant messages = %d, want only the adena line", len(msgs))
	}
	assertGrantMessage(t, msgs[0], serverpackets.SystemMessageEarnedS1Adena, numberParam(autoLootAdena))
	if drops := srv.GroundItems.Snapshots(nil); len(drops) != 1 || drops[0].TemplateID != autoLootWeaponID {
		t.Fatalf("ground drops = %+v, want the one weapon that did not fit", drops)
	}
}

// TestAutoLootEquipsArrowsForWornBow: auto-looted arrows go on a bow user's
// empty left hand.
func TestAutoLootEquipsArrowsForWornBow(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAutoLoot(true),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	bow := srv.GiveItem(t, objID, fixtureBowID, 1)
	startInWorld(t, c)
	c.Send(encodeUseItem(bow, false))
	drainUntilQuiet(t, c)
	inventoryUpdateAfterTick(t, srv, c)
	drainUntilQuiet(t, c)

	frames := killForAutoLoot(t, srv, objID, grantMonsterTemplate(item.Drop{ItemID: fixtureArrowID, Min: 20, Max: 20, Chance: 100}))
	msgs := grantMessages(t, frames)
	if len(msgs) != 1 {
		t.Fatalf("grant messages = %d, want 1", len(msgs))
	}
	assertGrantMessage(t, msgs[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(fixtureArrowID), itemNumberParam(20))
	arrows := wornLeftHand(t, srv, objID)
	if arrows == nil || arrows.TemplateID != fixtureArrowID {
		t.Fatalf("left hand = %+v, want the auto-looted arrows", arrows)
	}
	assertWorn(t, srv, objID, arrows.ObjectID, itemcontainer.LHand)
}
