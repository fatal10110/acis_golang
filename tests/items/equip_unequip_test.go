package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestEquipUnequipRoundTrip equips the D-grade sword through UseItem,
// requires UserInfo plus the tick-driven InventoryUpdate and the paperdoll
// location in the persisted row, then unequips it through
// RequestUnEquipItem and requires the inventory slot restore in both the
// packet stream and the items row.
func TestEquipUnequipRoundTrip(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(weapon, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	e := readInventoryUpdateFor(t, c, weapon, 1)
	if e.equipped != 1 {
		t.Fatalf("equipped flag after equip = %d, want 1", e.equipped)
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, weapon); inst.Location != item.LocationPaperdoll {
		t.Fatalf("persisted weapon location after equip = %v, want paperdoll", inst.Location)
	}

	c.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "unequip UserInfo"), serverpackets.OpcodeUserInfo, "unequip UserInfo")
	assertSystemMessageItem(t, readSkippingEquipNoise(t, c, "disarmed message"), serverpackets.SystemMessageS1Disarmed, 30)
	srv.InventoryUpdates.Tick()
	e = readInventoryUpdateFor(t, c, weapon, 1)
	if e.equipped != 0 {
		t.Fatalf("equipped flag after unequip = %d, want 0", e.equipped)
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, weapon); inst.Location != item.LocationInventory {
		t.Fatalf("persisted weapon location after unequip = %v, want inventory", inst.Location)
	}
}

// TestUnequipMessageReflectsEnchantLevel walks the unequip message table:
// a plain item answers S1_DISARMED while an enchanted one answers
// EQUIPMENT_S1_S2_REMOVED carrying the enchant level and item name.
func TestUnequipMessageReflectsEnchantLevel(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enchant    int32
		wantMsg    int
		wantParams int32
	}{
		{name: "plain", enchant: 0, wantMsg: serverpackets.SystemMessageS1Disarmed, wantParams: 1},
		{name: "enchanted", enchant: 6, wantMsg: serverpackets.SystemMessageEquipmentS1S2Removed, wantParams: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			chest := srv.GiveItem(t, objID, 40, 1)
			if tc.enchant > 0 {
				inst := mustFindItem(t, srv, objID, chest)
				inst.EnchantLevel = int(tc.enchant)
				if err := srv.Items.Update(context.Background(), inst); err != nil {
					t.Fatalf("seed enchant level: %v", err)
				}
			}
			startInWorld(t, c)

			// Equip first so unequip has something on the paperdoll.
			c.Send(encodeUseItem(chest, false))
			assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
			srv.InventoryUpdates.Tick()
			readInventoryUpdateFor(t, c, chest, 1)

			c.Send(encodeRequestUnEquipItem(int32(item.SlotChest)))
			assertFrameOpcode(t, readSkippingEquipNoise(t, c, "unequip UserInfo"), serverpackets.OpcodeUserInfo, "unequip UserInfo")
			frame := readSkippingEquipNoise(t, c, "unequip SystemMessage")
			assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "unequip SystemMessage")
			r := wire.NewReader(frame[1:])
			if got := r.ReadInt32(); got != int32(tc.wantMsg) {
				t.Fatalf("unequip message id = %d, want %d", got, tc.wantMsg)
			}
			if params := r.ReadInt32(); params != tc.wantParams {
				t.Fatalf("unequip message params = %d, want %d", params, tc.wantParams)
			}
			if tc.enchant > 0 {
				if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
					t.Fatalf("param[0] type = %d, want number", typ)
				}
				if lvl := r.ReadInt32(); lvl != tc.enchant {
					t.Fatalf("param[0] enchant = %d, want %d", lvl, tc.enchant)
				}
			}
			if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
				t.Fatalf("item param type = %d, want item name", typ)
			}
			if id := r.ReadInt32(); id != 40 {
				t.Fatalf("item param id = %d, want 40", id)
			}
			srv.InventoryUpdates.Tick()
			readInventoryUpdateFor(t, c, chest, 1)
		})
	}
}

// TestUnequipEmptySlotIsRejected pins RequestUnEquipItem against an empty
// paperdoll slot: only ActionFailed answers.
func TestUnequipEmptySlotIsRejected(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)

	c.Send(encodeRequestUnEquipItem(int32(item.SlotChest)))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeActionFailed, "empty-slot unequip")
	testsupport.SyncBarrier(t, c, func() { c.Send(encodeRequestItemList()) }, serverpackets.OpcodeItemList)
}
