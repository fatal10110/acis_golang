package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: BowRodListener (BowRodListener.java:11-36), registered on every
// player inventory (PcInventory.java:43), ties the left hand to the right: a
// bow put in the right hand takes Inventory.findArrowForBow's arrows
// (Inventory.java:881-905) into the left, and a bow or fishing rod leaving
// the right hand clears the left. It fires inside setPaperdollItem, so a
// ChangeRecorderListener (added last, per call) records the left-hand
// change first: RequestUnEquipItem's message names unequipped[0]
// (RequestUnEquipItem.java:43-64), the arrows or lure.
//
// UseItem.java:170-181: arrows and lures are not equipable
// (ItemInstance.isEquipable, :325-328); a lure goes on over a fishing rod
// through setPaperdollItem and broadcastUserInfo alone, and arrows reach no
// handler.

const (
	fixtureBowID   int32 = 14
	fixtureArrowID int32 = 17
	fishingRodID   int32 = 6529
	lureID         int32 = 6519
	// shortBowID is a bow that, unlike the shared fixture bow, can be
	// dropped.
	shortBowID int32 = 13
)

// handSlotCatalog is the shared catalog plus a droppable bow, a fishing rod
// and a lure.
func handSlotCatalog() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		&item.Template{
			ID:            shortBowID,
			Name:          "Short Bow",
			Kind:          item.KindWeapon,
			Slot:          item.SlotLRHand,
			Duration:      -1,
			Dropable:      true,
			Destroyable:   true,
			DefaultAction: item.ActionEquip,
			Weapon:        &item.WeaponDetail{Type: item.WeaponBow, MPConsume: 1, ReuseDelay: 1500},
		},
		&item.Template{
			ID:            fishingRodID,
			Name:          "Baby Duck Rod",
			Kind:          item.KindWeapon,
			Slot:          item.SlotLRHand,
			Duration:      -1,
			Destroyable:   true,
			DefaultAction: item.ActionEquip,
			Weapon:        &item.WeaponDetail{Type: item.WeaponFishingRod},
		},
		&item.Template{
			ID:            lureID,
			Name:          "Green Colored Lure - Low Grade",
			Kind:          item.KindEtcItem,
			Slot:          item.SlotLHand,
			Duration:      -1,
			Stackable:     true,
			Destroyable:   true,
			DefaultAction: item.ActionEquip,
			EtcItem:       &item.EtcItemDetail{Type: item.EtcItemLure},
		},
	))
}

// inventoryUpdateAfterTick flushes the pending InventoryUpdate and returns
// its entries keyed by object id, skipping any frame ahead of it.
func inventoryUpdateAfterTick(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) map[int32]inventoryEntry {
	t.Helper()
	srv.InventoryUpdates.Tick()
	for range 20 {
		frame := c.Read()
		if frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		byID := map[int32]inventoryEntry{}
		for _, e := range readInventoryUpdateEntries(t, frame) {
			byID[e.objID] = e
		}
		return byID
	}
	t.Fatal("no InventoryUpdate within 20 frames")
	return nil
}

// assertWorn requires the persisted row of objectID to sit in paperdoll
// slot, or in the inventory when slot is negative.
func assertWorn(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32, slot int) {
	t.Helper()
	srv.FlushItems(t)
	inst := mustFindItem(t, srv, ownerID, objectID)
	if slot < 0 {
		if inst.Location != item.LocationInventory {
			t.Fatalf("item %d persisted at %v/%d, want the inventory", objectID, inst.Location, inst.LocationData)
		}
		return
	}
	if inst.Location != item.LocationPaperdoll || inst.LocationData != slot {
		t.Fatalf("item %d persisted at %v/%d, want paperdoll slot %d", objectID, inst.Location, inst.LocationData, slot)
	}
}

// systemMessages returns the SystemMessage frames among frames.
func systemMessages(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			out = append(out, f)
		}
	}
	return out
}

// bootBowAndArrows boots a player holding a bow and a stack of its arrows
// and equips the bow, reading through its InventoryUpdate.
func bootBowAndArrows(t *testing.T) (srv *gameservertest.Server, objID, bow, arrows int32, equip map[int32]inventoryEntry, equipFrames [][]byte) {
	t.Helper()
	return bootWornBow(t, fixtureBowID)
}

// bootWornBow is bootBowAndArrows over the bow template bowID.
func bootWornBow(t *testing.T, bowID int32) (srv *gameservertest.Server, objID, bow, arrows int32, equip map[int32]inventoryEntry, equipFrames [][]byte) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithItemTemplates(handSlotCatalog()),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID = srv.SoleObjectID(t)
	bow = srv.GiveItem(t, objID, bowID, 1)
	arrows = srv.GiveItem(t, objID, fixtureArrowID, 10)
	startInWorld(t, c)

	c.Send(encodeUseItem(bow, false))
	equipFrames = collectUntilQuiet(t, c)
	equip = inventoryUpdateAfterTick(t, srv, c)
	return srv, objID, bow, arrows, equip, equipFrames
}

// TestBowEquipPullsMatchingArrows: putting a bow on takes the held arrows of
// its grade into the left hand at once; the one InventoryUpdate carries both
// equipped, only the bow is announced, and both rows land in the paperdoll.
func TestBowEquipPullsMatchingArrows(t *testing.T) {
	t.Parallel()
	srv, objID, bow, arrows, equip, frames := bootBowAndArrows(t)

	if e, ok := equip[bow]; !ok || e.equipped != 1 {
		t.Fatalf("bow InventoryUpdate entry = %+v (present %v), want equipped", e, ok)
	}
	if e, ok := equip[arrows]; !ok || e.equipped != 1 {
		t.Fatalf("arrow InventoryUpdate entry = %+v (present %v), want equipped", e, ok)
	}
	msgs := systemMessages(frames)
	if len(msgs) != 1 {
		t.Fatalf("equip system messages = %d, want only the bow's", len(msgs))
	}
	assertSystemMessageItem(t, msgs[0], serverpackets.SystemMessageS1Equipped, fixtureBowID)
	assertWorn(t, srv, objID, bow, itemcontainer.RHand)
	assertWorn(t, srv, objID, arrows, itemcontainer.LHand)
}

// TestUnequipBowTakesArrowsOff: taking the bow off, by RequestUnEquipItem or
// by UseItem on it, takes its arrows off too. RequestUnEquipItem's message
// names the first change the paperdoll recorded, the arrows; UseItem's
// names the bow it was used on.
func TestUnequipBowTakesArrowsOff(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		send    func(bow int32) []byte
		namedID int32
	}{
		{"RequestUnEquipItem", func(int32) []byte { return encodeRequestUnEquipItem(int32(item.SlotLRHand)) }, fixtureArrowID},
		{"UseItem", func(bow int32) []byte { return encodeUseItem(bow, false) }, fixtureBowID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, bow, arrows, _, _ := bootBowAndArrows(t)
			c := srv.Client

			c.Send(tc.send(bow))
			msgs := systemMessages(collectUntilQuiet(t, c))
			if len(msgs) != 1 {
				t.Fatalf("unequip system messages = %d, want one", len(msgs))
			}
			assertSystemMessageItem(t, msgs[0], serverpackets.SystemMessageS1Disarmed, tc.namedID)
			off := inventoryUpdateAfterTick(t, srv, c)
			for _, id := range []int32{bow, arrows} {
				if e, ok := off[id]; !ok || e.equipped != 0 {
					t.Fatalf("InventoryUpdate entry for %d = %+v (present %v), want unequipped", id, e, ok)
				}
			}
			if srv.PlayerInventory(t, objID).ItemAt(itemcontainer.LHand) != nil {
				t.Fatal("left hand still holds the arrows after the bow came off")
			}
			assertWorn(t, srv, objID, bow, -1)
			assertWorn(t, srv, objID, arrows, -1)
		})
	}
}

// TestDropWornBowLeavesArrowsUnequipped: dropping the worn bow takes its
// arrows off with it; they stay in the inventory.
func TestDropWornBowLeavesArrowsUnequipped(t *testing.T) {
	t.Parallel()
	srv, objID, bow, arrows, _, _ := bootWornBow(t, shortBowID)
	c := srv.Client

	c.Send(encodeRequestDropItem(bow, 1, spawnX, spawnY, spawnZ))
	frames := collectUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	frames = append(frames, collectUntilQuiet(t, c)...)
	off := map[int32]inventoryEntry{}
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readInventoryUpdateEntries(t, f) {
			off[e.objID] = e
		}
	}
	if e, ok := off[arrows]; !ok || e.equipped != 0 || e.state != 2 {
		t.Fatalf("arrow InventoryUpdate entry = %+v (present %v), want modified and unequipped", e, ok)
	}
	if e, ok := off[bow]; !ok || e.state != 3 {
		t.Fatalf("bow InventoryUpdate entry = %+v (present %v), want removed", e, ok)
	}
	assertWorn(t, srv, objID, arrows, -1)
	if got := len(srv.GroundItems.Snapshots(nil)); got != 1 {
		t.Fatalf("ground items after drop = %d, want the bow", got)
	}
}

// TestUseItemOnArrowsTogglesNothing: arrows are no equipment of their own,
// so UseItem on the worn ones answers ActionFailed and leaves them on.
func TestUseItemOnArrowsTogglesNothing(t *testing.T) {
	t.Parallel()
	srv, objID, _, arrows, _, _ := bootBowAndArrows(t)
	c := srv.Client

	c.Send(encodeUseItem(arrows, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "UseItem on arrows")
	assertNoFrameFor(t, c, 300*time.Millisecond, "after UseItem on arrows")
	if worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.LHand); worn == nil || worn.ObjectID != arrows {
		t.Fatalf("left hand after UseItem on arrows = %v, want the arrows", worn)
	}
}

// TestFishingRodLureGoesOnAndComesOffWithTheRod: a lure goes on only over a
// fishing rod, with no equip message, and taking the rod off takes the lure
// off too. RequestUnEquipItem names the lure, the first item it took off;
// UseItem names the rod it was sent for.
func TestFishingRodLureGoesOnAndComesOffWithTheRod(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		send    func(rod int32) []byte
		namedID int32
	}{
		{"RequestUnEquipItem", func(int32) []byte { return encodeRequestUnEquipItem(int32(item.SlotLRHand)) }, lureID},
		{"UseItem", func(rod int32) []byte { return encodeUseItem(rod, false) }, fishingRodID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, rod, lure := bootRodWithLure(t)
			c := srv.Client

			c.Send(tc.send(rod))
			msgs := systemMessages(collectUntilQuiet(t, c))
			if len(msgs) != 1 {
				t.Fatalf("rod unequip system messages = %d, want one", len(msgs))
			}
			assertSystemMessageItem(t, msgs[0], serverpackets.SystemMessageS1Disarmed, tc.namedID)
			off := inventoryUpdateAfterTick(t, srv, c)
			for _, id := range []int32{rod, lure} {
				if e, ok := off[id]; !ok || e.equipped != 0 {
					t.Fatalf("InventoryUpdate entry for %d = %+v (present %v), want unequipped", id, e, ok)
				}
			}
			if srv.PlayerInventory(t, objID).ItemAt(itemcontainer.LHand) != nil {
				t.Fatal("left hand still holds the lure after the rod came off")
			}
			assertWorn(t, srv, objID, rod, -1)
			assertWorn(t, srv, objID, lure, -1)
			if inst := mustFindItem(t, srv, objID, lure); inst.Count != 5 {
				t.Fatalf("lure count after the rod came off = %d, want 5", inst.Count)
			}
		})
	}
}

// bootRodWithLure boots a player wearing a fishing rod with a lure over it,
// pinning on the way that a lure is refused without a rod and goes on over
// one with a UserInfo and no system message.
func bootRodWithLure(t *testing.T) (srv *gameservertest.Server, objID, rod, lure int32) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithItemTemplates(handSlotCatalog()),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID = srv.SoleObjectID(t)
	rod = srv.GiveItem(t, objID, fishingRodID, 1)
	lure = srv.GiveItem(t, objID, lureID, 5)
	startInWorld(t, c)

	c.Send(encodeUseItem(lure, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "lure without a rod")
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(rod, false))
	drainUntilQuiet(t, c)
	if e := inventoryUpdateAfterTick(t, srv, c)[rod]; e.equipped != 1 {
		t.Fatalf("rod InventoryUpdate entry = %+v, want equipped", e)
	}
	if srv.PlayerInventory(t, objID).ItemAt(itemcontainer.LHand) != nil {
		t.Fatal("a rod pulled something into the left hand")
	}

	c.Send(encodeUseItem(lure, false))
	frames := collectUntilQuiet(t, c)
	if msgs := systemMessages(frames); len(msgs) != 0 {
		t.Fatalf("lure put on with %d system messages, want none", len(msgs))
	}
	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("lure put on without a leading UserInfo: %d frames", len(frames))
	}
	if e := inventoryUpdateAfterTick(t, srv, c)[lure]; e.equipped != 1 {
		t.Fatalf("lure InventoryUpdate entry = %+v, want equipped", e)
	}
	return srv, objID, rod, lure
}

// assertPickedUpCrystals asserts frame is YOU_PICKED_UP_S2_S1 naming count
// crystals of crystalID.
func assertPickedUpCrystals(t *testing.T, frame []byte, crystalID, count int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "crystal pickup SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageYouPickedUpS2S1 {
		t.Fatalf("message id = %d, want YOU_PICKED_UP_S2_S1 (%d)", id, serverpackets.SystemMessageYouPickedUpS2S1)
	}
	if n := r.ReadInt32(); n != 2 {
		t.Fatalf("param count = %d, want 2", n)
	}
	if typ, id := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName || id != crystalID {
		t.Fatalf("param[0] = (%d, %d), want item name %d", typ, id, crystalID)
	}
	if typ, n := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamItemNumber || n != count {
		t.Fatalf("param[1] = (%d, %d), want item number %d", typ, n, count)
	}
}

// TestCrystallizeInStoreModeIsRefused pins RequestCrystallizeItem.java:36-40:
// a player running a private store is answered
// CANNOT_TRADE_DISCARD_DROP_ITEM_WHILE_IN_SHOPMODE and keeps the item.
func TestCrystallizeInStoreModeIsRefused(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, 248, 3); err != nil {
		t.Fatalf("grant crystallize skill: %v", err)
	}
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)
	srv.SetPlayerOperating(t, objID, true)

	c.Send(encodeRequestCrystallizeItem(weapon, 1))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotTradeDiscardDropInShopMode)
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the store-mode refusal")
	if inst := mustFindItem(t, srv, objID, weapon); inst.Count != 1 {
		t.Fatalf("weapon count after refused crystallize = %d, want 1", inst.Count)
	}
}

// onlineLivePlayer returns objID's online character.
func onlineLivePlayer(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an online character", objID, obj)
	}
	return ch
}
