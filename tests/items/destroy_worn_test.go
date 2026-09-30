package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: RequestDestroyItem.java:60-61 takes a worn item off through
// Player.useEquippableItem(item, false) (Player.java:1210-1252) when the
// destroy consumes all of it: a weapon's shots are uncharged, the removal
// message (EQUIPMENT_S1_S2_REMOVED with the enchant level, or S1_DISARMED)
// goes out ahead of the paperdoll change, then refreshExpertisePenalty,
// broadcastUserInfo, and ExStorageMaxCount when the inventory limit moved.
// Only then does Player.destroyItem consume the item.

// TestDestroyWornWeaponAnnouncesRemoval: destroying a worn sword names it
// with S1_DISARMED, an enchanted one with EQUIPMENT_S1_S2_REMOVED carrying
// the enchant level, ahead of the UserInfo refresh; the row is gone after.
func TestDestroyWornWeaponAnnouncesRemoval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		enchant int
		wantMsg int
	}{
		{name: "plain", enchant: 0, wantMsg: serverpackets.SystemMessageS1Disarmed},
		{name: "enchanted", enchant: 4, wantMsg: serverpackets.SystemMessageEquipmentS1S2Removed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			sword := srv.GiveItem(t, objID, 30, 1)
			if tc.enchant > 0 {
				inst := mustFindItem(t, srv, objID, sword)
				inst.EnchantLevel = tc.enchant
				if err := srv.Items.Update(context.Background(), inst); err != nil {
					t.Fatalf("seed enchant level: %v", err)
				}
			}
			startInWorld(t, c)
			wearAndSettle(t, srv, c, sword)

			c.Send(encodeRequestDestroyItem(sword, 1))
			frames := collectUntilQuiet(t, c)
			if len(frames) == 0 {
				t.Fatal("destroying the worn sword sent nothing")
			}
			assertRemovalMessage(t, frames[0], tc.wantMsg, tc.enchant, 30)
			if userInfoFrame(frames) < 0 {
				t.Fatal("destroying the worn sword sent no UserInfo refresh")
			}
			for _, f := range frames {
				if f[0] == serverpackets.OpcodeActionFailed {
					t.Fatal("destroying the worn sword answered ActionFailed")
				}
			}
			srv.InventoryUpdates.Tick()
			drainUntilQuiet(t, c)
			srv.FlushItems(t)
			assertItemGone(t, srv, objID, sword)
		})
	}
}

// TestDestroyWornOverGradeWeaponClearsGradePenalty: a player without
// Expertise wearing the D-grade sword carries the weapon grade penalty;
// destroying the sword refreshes it, so an EtcStatusUpdate without the
// penalty reaches the client ahead of the UserInfo refresh.
func TestDestroyWornOverGradeWeaponClearsGradePenalty(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(sword, false))
	equipFrames := collectUntilQuiet(t, c)
	if etc := etcStatusFrames(equipFrames); len(etc) != 1 || !etcStatusGradePenalty(t, etc[0]) {
		t.Fatalf("equip EtcStatusUpdate frames = %d, want one showing the grade penalty", len(etc))
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)

	c.Send(encodeRequestDestroyItem(sword, 1))
	frames := collectUntilQuiet(t, c)
	etc := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeEtcStatusUpdate {
			etc = i
			break
		}
	}
	if etc < 0 {
		t.Fatal("destroying the worn over-grade sword sent no EtcStatusUpdate")
	}
	if etcStatusGradePenalty(t, frames[etc]) {
		t.Fatal("EtcStatusUpdate after destroying the worn sword still shows the grade penalty")
	}
	if ui := userInfoFrame(frames); ui < etc {
		t.Fatalf("UserInfo at frame %d, EtcStatusUpdate at %d: want the penalty refresh first", ui, etc)
	}
}

// TestDestroyWornItemResendsStorageLimit: destroying worn armor whose
// inventoryLimit bonus raised the limit sends ExStorageMaxCount with the
// lowered limit after the UserInfo refresh.
func TestDestroyWornItemResendsStorageLimit(t *testing.T) {
	t.Parallel()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == inventoryLimitArmorID {
			clone := *tmpl
			clone.Modifiers = append(append([]item.StatModifier(nil), tmpl.Modifiers...),
				item.StatModifier{Op: item.FuncAdd, Stat: "inventoryLimit", Value: 5})
			tmpl = &clone
		}
		templates = append(templates, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	armor := srv.GiveItem(t, objID, inventoryLimitArmorID, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(armor, false))
	assertStorageLimitAfterUserInfo(t, collectUntilQuiet(t, c), 85)
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)

	c.Send(encodeRequestDestroyItem(armor, 1))
	frames := collectUntilQuiet(t, c)
	assertRemovalMessage(t, frames[0], serverpackets.SystemMessageS1Disarmed, 0, inventoryLimitArmorID)
	assertStorageLimitAfterUserInfo(t, frames, 80)
}

// TestDestroyWornArrowsOnlyWhenWholeStackGoes: destroying part of the worn
// arrow stack leaves the rest worn with no removal message; destroying the
// rest takes the arrows off first, naming them, while the bow stays worn.
func TestDestroyWornArrowsOnlyWhenWholeStackGoes(t *testing.T) {
	t.Parallel()
	srv, objID, bow, arrows, _, _ := bootBowAndArrows(t)
	c := srv.Client

	c.Send(encodeRequestDestroyItem(arrows, 4))
	for _, f := range collectUntilQuiet(t, c) {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageS1Disarmed {
			t.Fatal("destroying part of the worn arrows announced a removal")
		}
		if f[0] == serverpackets.OpcodeUserInfo {
			t.Fatal("destroying part of the worn arrows refreshed UserInfo")
		}
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	assertWorn(t, srv, objID, arrows, itemcontainer.LHand)

	c.Send(encodeRequestDestroyItem(arrows, 6))
	frames := collectUntilQuiet(t, c)
	assertRemovalMessage(t, frames[0], serverpackets.SystemMessageS1Disarmed, 0, fixtureArrowID)
	if userInfoFrame(frames) < 0 {
		t.Fatal("destroying the worn arrows sent no UserInfo refresh")
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	assertWorn(t, srv, objID, bow, itemcontainer.RHand)
	assertItemGone(t, srv, objID, arrows)
}

// wearAndSettle equips objectID and reads through its refresh and
// InventoryUpdate.
func wearAndSettle(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objectID int32) {
	t.Helper()
	c.Send(encodeUseItem(objectID, false))
	readSkippingEquipNoise(t, c, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, objectID, 1)
	drainUntilQuiet(t, c)
}

// assertRemovalMessage asserts an unequip announcement: messageID naming
// itemID, preceded by the enchant level when enchant is positive.
func assertRemovalMessage(t *testing.T, frame []byte, messageID, enchant int, itemID int32) {
	t.Helper()
	if enchant == 0 {
		assertSystemMessageItem(t, frame, messageID, itemID)
		return
	}
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "removal SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("removal message id = %d, want %d", got, messageID)
	}
	if params := r.ReadInt32(); params != 2 {
		t.Fatalf("removal message params = %d, want 2", params)
	}
	if typ, lvl := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber || lvl != int32(enchant) {
		t.Fatalf("removal message param[0] = type %d value %d, want number %d", typ, lvl, enchant)
	}
	if typ, id := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName || id != itemID {
		t.Fatalf("removal message param[1] = type %d value %d, want item name %d", typ, id, itemID)
	}
}

func etcStatusFrames(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeEtcStatusUpdate {
			out = append(out, f)
		}
	}
	return out
}

// etcStatusGradePenalty decodes the grade-penalty flag of an
// EtcStatusUpdate frame.
func etcStatusGradePenalty(t *testing.T, frame []byte) bool {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeEtcStatusUpdate, "EtcStatusUpdate")
	r := wire.NewReader(frame[1:])
	for range 4 { // charges, weight penalty, blocked, danger area
		r.ReadInt32()
	}
	gradePenalty := r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read EtcStatusUpdate: %v", err)
	}
	return gradePenalty != 0
}
