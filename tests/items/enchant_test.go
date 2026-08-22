package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// assertItemGone fails unless no persisted row for objectID remains.
func assertItemGone(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) {
	t.Helper()
	for _, inst := range persistedItems(t, srv, ownerID) {
		if inst.ObjectID == objectID {
			t.Fatalf("item row %d still persisted: template %d count %d", objectID, inst.TemplateID, inst.Count)
		}
	}
}

// assertChooseInventoryItem asserts a ChooseInventoryItem(itemID) packet.
func assertChooseInventoryItem(t *testing.T, frame []byte, itemID int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeChooseInventoryItem, "ChooseInventoryItem")
	if len(frame) != 5 {
		t.Fatalf("ChooseInventoryItem frame = %x", frame)
	}
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != itemID {
		t.Fatalf("ChooseInventoryItem item id = %d, want %d", got, itemID)
	}
}

// assertEnchantResult asserts an EnchantResult(result) packet.
func assertEnchantResult(t *testing.T, frame []byte, result serverpackets.EnchantResult) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeEnchantResult, "EnchantResult")
	if len(frame) != 5 {
		t.Fatalf("EnchantResult frame = %x", frame)
	}
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != int32(result) {
		t.Fatalf("EnchantResult result = %d, want %d", got, result)
	}
}

// openEnchantSelection uses the scroll from the item window and requires the
// selection prompt pair.
func openEnchantSelection(t *testing.T, c *testsupport.ScriptedClient, scrollObj, scrollTemplate int32) {
	t.Helper()
	c.Send(encodeUseItem(scrollObj, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSelectItemToEnchant)
	assertChooseInventoryItem(t, c.Read(), scrollTemplate)
}

// bootEnchanter boots a character holding a weapon at the given enchant
// level plus one weapon enchant scroll (blessed when requested). extra runs
// before the character enters the world, so anything it seeds is part of the
// loaded inventory.
func bootEnchanter(t *testing.T, roll func() float64, weaponEnchant int32, blessed bool, extra func(t *testing.T, srv *gameservertest.Server, objID int32)) (*gameservertest.Server, int32, int32, int32) {
	t.Helper()
	opts := []gameservertest.Option{
		gameservertest.WithEnchantRoll(roll),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	}
	srv := gameservertest.Boot(t, opts...)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	if weaponEnchant > 0 {
		inst := mustFindItem(t, srv, objID, weapon)
		inst.EnchantLevel = int(weaponEnchant)
		if err := srv.Items.Update(context.Background(), inst); err != nil {
			t.Fatalf("seed enchant level: %v", err)
		}
	}
	scrollTemplate := int32(955)
	if blessed {
		scrollTemplate = 6575
	}
	scroll := srv.GiveItem(t, objID, scrollTemplate, 1)
	if extra != nil {
		extra(t, srv, objID)
	}
	startInWorld(t, c)
	return srv, objID, weapon, scroll
}

// TestEnchantScrollFlow walks the enchant table: a normal scroll succeeds on
// a winning roll and persists the level; on a losing roll at +3 the weapon
// breaks into crystals; a blessed scroll that loses resets the level and
// keeps the weapon; and an invalid target cancels without consuming the
// scroll. Each branch is asserted on packets, world-independent inventory
// state, and the persisted rows.
func TestEnchantScrollFlow(t *testing.T) {
	t.Run("success persists enchant level", func(t *testing.T) {
		srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0.0 }, 0, false, nil)
		c := srv.Client

		openEnchantSelection(t, c, scroll, 955)
		c.Send(encodeRequestEnchantItem(weapon))

		frame := c.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "success SystemMessage")
		if id := systemMessageID(t, frame); id != serverpackets.SystemMessageS1SuccessfullyEnchanted {
			t.Fatalf("message id = %d, want S1SuccessfullyEnchanted (%d)", id, serverpackets.SystemMessageS1SuccessfullyEnchanted)
		}
		assertEnchantResult(t, c.Read(), serverpackets.EnchantResultSuccess)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "success UserInfo")
		srv.InventoryUpdates.Tick()
		entries := readInventoryUpdateEntries(t, c.Read())
		if len(entries) != 2 {
			t.Fatalf("success InventoryUpdate entries = %+v, want consumed scroll plus enchanted weapon", entries)
		}
		scrollEntry, weaponEntry := entries[0], entries[1]
		if scrollEntry.state != 3 || scrollEntry.objID != scroll {
			t.Fatalf("first success entry = %+v, want removed scroll %d", scrollEntry, scroll)
		}
		if weaponEntry.state != 2 || weaponEntry.objID != weapon || weaponEntry.enchant != 1 {
			t.Fatalf("second success entry = %+v, want modified weapon %d at enchant 1", weaponEntry, weapon)
		}

		inst := mustFindItem(t, srv, objID, weapon)
		if inst.EnchantLevel != 1 {
			t.Fatalf("persisted enchant level = %d, want 1", inst.EnchantLevel)
		}
		assertItemGone(t, srv, objID, scroll)
	})

	t.Run("failure breaks weapon into crystals", func(t *testing.T) {
		srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0.99 }, 3, false, nil)
		c := srv.Client

		openEnchantSelection(t, c, scroll, 955)
		c.Send(encodeRequestEnchantItem(weapon))

		frame := c.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "crystal reward SystemMessage")
		if id := systemMessageID(t, frame); id != serverpackets.SystemMessageEarnedS2S1S {
			t.Fatalf("first message id = %d, want EarnedS2S1S (%d)", id, serverpackets.SystemMessageEarnedS2S1S)
		}
		frame = c.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "evaporated SystemMessage")
		if id := systemMessageID(t, frame); id != serverpackets.SystemMessageEnchantmentFailedS1S2Evaporated {
			t.Fatalf("second message id = %d, want EnchantmentFailedS1S2Evaporated (%d)", id, serverpackets.SystemMessageEnchantmentFailedS1S2Evaporated)
		}
		assertEnchantResult(t, c.Read(), serverpackets.EnchantResultBrokenWithCrystals)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "failure UserInfo")
		srv.InventoryUpdates.Tick()
		entries := readInventoryUpdateEntries(t, c.Read())
		var removedWeapon, addedCrystal bool
		for _, e := range entries {
			if e.state == 3 && e.objID == weapon {
				removedWeapon = true
			}
			if e.state == 1 && e.itemID == item.CrystalD.ItemID() {
				addedCrystal = true
			}
		}
		if !removedWeapon || !addedCrystal {
			t.Fatalf("failure InventoryUpdate entries = %+v, want removed weapon plus added D crystals", entries)
		}

		assertItemGone(t, srv, objID, weapon)
		assertItemGone(t, srv, objID, scroll)
		crystalCount := 0
		for _, inst := range persistedItems(t, srv, objID) {
			if inst.TemplateID == item.CrystalD.ItemID() {
				crystalCount += inst.Count
			}
		}
		if crystalCount == 0 {
			t.Fatal("no persisted crystal reward after the weapon broke")
		}
	})

	t.Run("blessed failure resets enchant level", func(t *testing.T) {
		srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0.99 }, 3, true, nil)
		c := srv.Client

		openEnchantSelection(t, c, scroll, 6575)
		c.Send(encodeRequestEnchantItem(weapon))

		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageBlessedEnchantFailed)
		assertEnchantResult(t, c.Read(), serverpackets.EnchantResultUnsuccess)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "blessed UserInfo")
		srv.InventoryUpdates.Tick()
		entries := readInventoryUpdateEntries(t, c.Read())
		if len(entries) != 2 {
			t.Fatalf("blessed InventoryUpdate entries = %+v, want consumed scroll plus reset weapon", entries)
		}
		scrollEntry, weaponEntry := entries[0], entries[1]
		if scrollEntry.state != 3 || scrollEntry.objID != scroll {
			t.Fatalf("first blessed entry = %+v, want removed scroll %d", scrollEntry, scroll)
		}
		if weaponEntry.state != 2 || weaponEntry.objID != weapon || weaponEntry.enchant != 0 {
			t.Fatalf("second blessed entry = %+v, want modified weapon %d at enchant 0", weaponEntry, weapon)
		}

		if inst := mustFindItem(t, srv, objID, weapon); inst.EnchantLevel != 0 {
			t.Fatalf("persisted enchant level after blessed failure = %d, want 0", inst.EnchantLevel)
		}
		assertItemGone(t, srv, objID, scroll)
	})

	t.Run("invalid target cancels without consuming", func(t *testing.T) {
		srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0.0 }, 0, false, func(t *testing.T, srv *gameservertest.Server, objID int32) {
			srv.GiveItem(t, objID, item.CrystalD.ItemID(), 5)
		})
		c := srv.Client
		crystal := mustFindItemByTemplate(t, srv, objID, item.CrystalD.ItemID())

		openEnchantSelection(t, c, scroll, 955)
		c.Send(encodeRequestEnchantItem(crystal.ObjectID))

		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInappropriateEnchantCondition)
		assertEnchantResult(t, c.Read(), serverpackets.EnchantResultCancelled)
		barrier(t, c)

		if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 1 {
			t.Fatalf("scroll count after cancelled enchant = %d, want 1", inst.Count)
		}
		if inst := mustFindItem(t, srv, objID, weapon); inst.EnchantLevel != 0 {
			t.Fatalf("weapon enchant after cancelled enchant = %d, want 0", inst.EnchantLevel)
		}
	})
}

// TestEnchantRequestWithoutSelectionIsSilent pins the missing-selection
// branch: RequestEnchantItem with no scroll opened produces no reply at all.
func TestEnchantRequestWithoutSelectionIsSilent(t *testing.T) {
	srv, _, weapon, _ := bootEnchanter(t, func() float64 { return 0.0 }, 0, false, nil)
	c := srv.Client
	drainUntilQuiet(t, c)

	c.Send(encodeRequestEnchantItem(weapon))
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("enchant without active scroll replied %x, want no reply at all", reply)
	}
}
