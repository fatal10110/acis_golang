package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: PcInventory.removeItem (PcInventory.java:484-502) deletes every
// ITEM shortcut naming an instance that leaves the inventory, through
// ShortcutList.deleteShortcuts -> deleteShortcut (ShortcutList.java:127-151):
// the row goes, ShortCutDelete(slot) goes out, then ExAutoSoulShot(id, 1) for
// every shot still on automatic use. The instance has already left
// ItemContainer._items by then (ItemContainer.removeItem), so the "shot still
// held" branch that turns automatic use off never fires on this path; it does
// on a client RequestShortCutDel of a held shot.

// shortcutPage and shortcutSlot place every test shortcut on bar page 1.
const (
	shortcutPage int32 = 1
	shortcutSlot int32 = 4
)

func encodeRequestShortCutReg(typ, slot, id, characterType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutReg)
	w.WriteInt32(typ)
	w.WriteInt32(slot)
	w.WriteInt32(id)
	w.WriteInt32(characterType)
	return w.Bytes()
}

func encodeRequestShortCutDel(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShortCutDel)
	w.WriteInt32(slot)
	return w.Bytes()
}

// registerItemShortcut puts objectID on the bar at the test slot and waits
// for the row to land.
func registerItemShortcut(t *testing.T, srv *gameservertest.Server, objectID int32) {
	t.Helper()
	srv.Client.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), shortcutPage*12+shortcutSlot, objectID, 1))
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeShortCutRegister, "ShortCutRegister")
	srv.FlushPersistence(t)
	if !hasItemShortcutRow(t, srv, objectID) {
		t.Fatalf("no shortcut row for item %d after registration", objectID)
	}
}

func hasItemShortcutRow(t *testing.T, srv *gameservertest.Server, objectID int32) bool {
	t.Helper()
	rows, err := srv.Shortcuts.ListByOwner(context.Background(), srv.SoleObjectID(t))
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	for _, row := range rows {
		if row.Type == shortcut.Item && row.ID == objectID {
			return true
		}
	}
	return false
}

// shortCutDeleteSlot returns the wire slot of a ShortCutDelete frame, or
// false for any other frame.
func shortCutDeleteSlot(frame []byte) (int32, bool) {
	if frame[0] != serverpackets.OpcodeShortCutDelete {
		return 0, false
	}
	return wire.NewReader(frame[1:]).ReadInt32(), true
}

// exAutoSoulShot decodes an ExAutoSoulShot frame.
func exAutoSoulShot(frame []byte) (itemID int32, enabled, ok bool) {
	if frame[0] != serverpackets.OpcodeExtended {
		return 0, false, false
	}
	r := wire.NewReader(frame[1:])
	if r.ReadUint16() != serverpackets.OpcodeExAutoSoulShot {
		return 0, false, false
	}
	itemID = r.ReadInt32()
	return itemID, r.ReadInt32() == 1, true
}

// shortcutFrames returns the indexes of the ShortCutDelete frames and the
// ExAutoSoulShot frames among frames.
func shortcutFrames(t *testing.T, frames [][]byte) (deletes []int, autoShots []int) {
	t.Helper()
	for i, f := range frames {
		if slot, ok := shortCutDeleteSlot(f); ok {
			if slot != shortcutPage*12+shortcutSlot {
				t.Fatalf("ShortCutDelete slot = %d, want %d", slot, shortcutPage*12+shortcutSlot)
			}
			deletes = append(deletes, i)
		}
		if _, _, ok := exAutoSoulShot(f); ok {
			autoShots = append(autoShots, i)
		}
	}
	return deletes, autoShots
}

// TestItemLeavingInventoryDeletesItsShortcut destroys, drops and
// crystallizes an item on the bar: each sends ShortCutDelete for its slot
// and deletes the row, so the next login restores nothing for it.
func TestItemLeavingInventoryDeletesItsShortcut(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		remove func(objectID int32) []byte
	}{
		{"destroy", func(objectID int32) []byte { return encodeRequestDestroyItem(objectID, 1) }},
		{"drop", func(objectID int32) []byte { return encodeRequestDropItem(objectID, 1, spawnX, spawnY, spawnZ) }},
		{"crystallize", func(objectID int32) []byte { return encodeRequestCrystallizeItem(objectID, 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, 248, 3); err != nil {
				t.Fatalf("grant crystallize skill: %v", err)
			}
			sword := srv.GiveItem(t, objID, 30, 1)
			startInWorld(t, c)
			registerItemShortcut(t, srv, sword)

			c.Send(tc.remove(sword))
			deletes, autoShots := shortcutFrames(t, collectUntilQuiet(t, c))
			if len(deletes) != 1 {
				t.Fatalf("ShortCutDelete frames = %d, want 1", len(deletes))
			}
			if len(autoShots) != 0 {
				t.Fatalf("ExAutoSoulShot frames = %d with no automatic shot on, want 0", len(autoShots))
			}
			srv.FlushPersistence(t)
			if hasItemShortcutRow(t, srv, sword) {
				t.Fatalf("shortcut row for item %d survived its %s", sword, tc.name)
			}
		})
	}
}

// TestPartialStackRemovalKeepsShortcut destroys part of a stack on the bar:
// the instance stays, so its shortcut and row do too.
func TestPartialStackRemovalKeepsShortcut(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potions := srv.GiveItem(t, objID, 20, 5)
	startInWorld(t, c)
	registerItemShortcut(t, srv, potions)

	c.Send(encodeRequestDestroyItem(potions, 2))
	if deletes, _ := shortcutFrames(t, collectUntilQuiet(t, c)); len(deletes) != 0 {
		t.Fatalf("ShortCutDelete frames after a partial destroy = %d, want 0", len(deletes))
	}
	srv.FlushPersistence(t)
	if !hasItemShortcutRow(t, srv, potions) {
		t.Fatal("partial destroy deleted the stack's shortcut row")
	}
}

// TestLastShotStackRemovalReannouncesAutoShots destroys the whole spiritshot
// stack on the bar while it is on automatic use. The shortcut goes like any
// other, followed by ExAutoSoulShot(id, 1) for the shot still on automatic
// use: the removed instance is no longer held when the shortcut is deleted,
// so nothing turns automatic use off here.
func TestLastShotStackRemovalReannouncesAutoShots(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	shots := srv.GiveItem(t, objID, autoSpiritshotID, 3)
	startInWorld(t, c)
	registerItemShortcut(t, srv, shots)
	c.Send(encodeRequestAutoSoulShot(autoSpiritshotID, 1))
	drainUntilQuiet(t, c)

	c.Send(encodeRequestDestroyItem(shots, 3))
	frames := collectUntilQuiet(t, c)
	deletes, autoShots := shortcutFrames(t, frames)
	if len(deletes) != 1 || len(autoShots) != 1 || autoShots[0] < deletes[0] {
		t.Fatalf("ShortCutDelete at %v, ExAutoSoulShot at %v, want one ShortCutDelete then one ExAutoSoulShot", deletes, autoShots)
	}
	assertExAutoSoulShot(t, frames[autoShots[0]], autoSpiritshotID, true)
	srv.FlushPersistence(t)
	if hasItemShortcutRow(t, srv, shots) {
		t.Fatal("shortcut row for the destroyed shot stack survived")
	}
}

// TestDeletingHeldShotShortcutTurnsAutoUseOff deletes, from the client, the
// shortcut of a spiritshot stack still held and on automatic use: automatic
// use goes off with ExAutoSoulShot(id, 0) ahead of ShortCutDelete, and no
// shot is left to re-announce.
func TestDeletingHeldShotShortcutTurnsAutoUseOff(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	shots := srv.GiveItem(t, objID, autoSpiritshotID, 3)
	startInWorld(t, c)
	registerItemShortcut(t, srv, shots)
	c.Send(encodeRequestAutoSoulShot(autoSpiritshotID, 1))
	drainUntilQuiet(t, c)

	c.Send(encodeRequestShortCutDel(shortcutPage*12 + shortcutSlot))
	frames := collectUntilQuiet(t, c)
	if len(frames) != 2 {
		t.Fatalf("shortcut delete sent %d frames, want ExAutoSoulShot then ShortCutDelete", len(frames))
	}
	assertExAutoSoulShot(t, frames[0], autoSpiritshotID, false)
	if _, ok := shortCutDeleteSlot(frames[1]); !ok {
		t.Fatalf("second frame opcode = %#x, want ShortCutDelete", frames[1][0])
	}
	var enabled bool
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { enabled = pc.AutoSoulShotEnabled(autoSpiritshotID) })
	if enabled {
		t.Fatal("automatic spiritshot use still on after its shortcut was deleted")
	}
}
