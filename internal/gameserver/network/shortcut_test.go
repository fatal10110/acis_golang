//go:build integration

package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkEnterWorldSendsPersistedShortcuts(t *testing.T) {
	c, chars, shortcuts, _ := newLinkedSQLGameClientWithShortcuts(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)
	if err := shortcuts.Save(context.Background(), objID, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1}); err != nil {
		t.Fatalf("seed shortcut: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c, false)

	frame := frames[9]
	r := wire.NewReader(frame[1:])
	found := false
	for range r.ReadInt32() {
		typ, slot, id, characterType := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		found = found || typ == int32(serverpackets.ShortcutAction) && slot == 15 && id == 5 && characterType == 1
	}
	if !found {
		t.Fatal("ShortCutInit did not include persisted action slot 15 id 5")
	}
}

func TestGameClientLinkRegistersShortcut(t *testing.T) {
	c, chars, shortcuts, _ := newLinkedSQLGameClientWithShortcuts(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutAction), 15, 5, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}
	got, err := shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	want := shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1}
	if !hasShortcut(got, want) {
		t.Fatalf("shortcuts = %+v, want %+v", got, want)
	}
}

// TestGameClientLinkRejectsItemShortcutForObjectNotInInventory mirrors
// ShortcutList.addShortcut's ITEM branch (ShortcutList.java:62-98): a
// registration for an ITEM objectId not in the player's live inventory is
// silently dropped — no reply, no persisted row.
func TestGameClientLinkRejectsItemShortcutForObjectNotInInventory(t *testing.T) {
	const missingObjectID int32 = 999

	c, chars, _, shortcuts, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), 15, missingObjectID, 1))
	assertNoReply(t, c)

	got, err := shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	if hasShortcut(got, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Item, ID: missingObjectID, Level: -1, CharacterType: 1}) {
		t.Fatalf("shortcuts = %+v, want no row for missing item", got)
	}
}

// TestGameClientLinkRegistersItemShortcutForObjectInInventory is the
// success-path counterpart: an ITEM objectId present in inventory persists
// as today.
func TestGameClientLinkRegistersItemShortcutForObjectInInventory(t *testing.T) {
	const potionTemplate int32 = 9502 // Greater Healing Potion fixture
	const potionObjectID int32 = 700

	c, chars, items, shortcuts, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID: potionObjectID, TemplateID: potionTemplate, OwnerID: objID,
		Count: 1, Location: item.LocationInventory, ManaLeft: -1,
	}); err != nil {
		t.Fatalf("seed potion: %v", err)
	}
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), 15, potionObjectID, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}
	got, err := shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	want := shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Item, ID: potionObjectID, Level: -1, CharacterType: 1}
	if !hasShortcut(got, want) {
		t.Fatalf("shortcuts = %+v, want %+v", got, want)
	}
}

func TestGameClientLinkRegistersSkillShortcutAtKnownLevel(t *testing.T) {
	c, chars, shortcuts, knownSkills := newLinkedSQLGameClientWithShortcuts(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)
	if err := knownSkills.SetKnownSkill(context.Background(), objID, 0, 248, 3); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutSkill), 15, 248, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}
	r := wire.NewReader(reply[1:])
	if typ, slot, id, level, marker, characterType := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadUint8(), r.ReadInt32(); typ != int32(serverpackets.ShortcutSkill) || slot != 15 || id != 248 || level != 3 || marker != 0 || characterType != 1 {
		t.Fatalf("ShortCutRegister skill = type %d slot %d id %d level %d marker %d charType %d, want skill slot 15 id 248 level 3 marker 0 charType 1", typ, slot, id, level, marker, characterType)
	}
	got, err := shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	want := shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Skill, ID: 248, Level: 3, CharacterType: 1}
	if !hasShortcut(got, want) {
		t.Fatalf("shortcuts = %+v, want %+v", got, want)
	}
}

func TestGameClientLinkDeletesShortcut(t *testing.T) {
	c, chars, shortcuts, _ := newLinkedSQLGameClientWithShortcuts(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)
	if err := shortcuts.Save(context.Background(), objID, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1}); err != nil {
		t.Fatalf("seed shortcut: %v", err)
	}
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeRequestShortCutDel(15))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeShortCutDelete {
		t.Fatalf("opcode = %#x, want ShortCutDelete (%#x)", reply[0], serverpackets.OpcodeShortCutDelete)
	}
	got, err := shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	if hasShortcut(got, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1}) {
		t.Fatalf("shortcuts after delete = %+v, still contains deleted shortcut", got)
	}
}

// TestGameClientLinkEnterWorldDropsStaleItemShortcutAndSetsSharedReuseGroup
// mirrors ShortcutList.restore() (ShortcutList.java:173-209): a persisted
// ITEM shortcut whose item no longer exists in the inventory is dropped, and
// a surviving one has SharedReuseGroup populated from the item's etc-item
// data instead of the hardcoded -1.
func TestGameClientLinkEnterWorldDropsStaleItemShortcutAndSetsSharedReuseGroup(t *testing.T) {
	const potionTemplate int32 = 9502 // Greater Healing Potion fixture, shared_reuse_group 10
	const potionObjectID int32 = 700
	const staleObjectID int32 = 999

	c, chars, items, shortcuts, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)

	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID: potionObjectID, TemplateID: potionTemplate, OwnerID: objID,
		Count: 1, Location: item.LocationInventory, ManaLeft: -1,
	}); err != nil {
		t.Fatalf("seed potion: %v", err)
	}
	if err := shortcuts.Save(context.Background(), objID, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Item, ID: potionObjectID, Level: -1, CharacterType: 1, SharedReuseGroup: -1}); err != nil {
		t.Fatalf("seed live item shortcut: %v", err)
	}
	if err := shortcuts.Save(context.Background(), objID, shortcut.Shortcut{Slot: 4, Page: 1, Type: shortcut.Item, ID: staleObjectID, Level: -1, CharacterType: 1, SharedReuseGroup: -1}); err != nil {
		t.Fatalf("seed stale item shortcut: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c, false)

	frame := frames[9]
	r := wire.NewReader(frame[1:])
	var foundLive, foundStale bool
	for range r.ReadInt32() {
		typ, slot := r.ReadInt32(), r.ReadInt32()
		if typ != int32(serverpackets.ShortcutItem) {
			switch serverpackets.ShortcutType(typ) {
			case serverpackets.ShortcutSkill:
				r.ReadInt32()
				r.ReadInt32()
				r.ReadUint8()
				r.ReadInt32()
			default:
				r.ReadInt32()
				r.ReadInt32()
			}
			continue
		}
		id, characterType, group, remaining, reuse, augment := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		_, _, _ = remaining, reuse, augment
		if id == potionObjectID {
			foundLive = true
			if slot != 15 || characterType != 1 || group != 10 {
				t.Fatalf("live item shortcut = slot %d charType %d group %d, want slot 15 charType 1 group 10", slot, characterType, group)
			}
		}
		if id == staleObjectID {
			foundStale = true
		}
	}
	if !foundLive {
		t.Fatal("ShortCutInit did not include the live item shortcut")
	}
	if foundStale {
		t.Fatal("ShortCutInit still includes the stale item shortcut, want dropped on restore")
	}
}

func sqlCharacterID(t *testing.T, chars *gamesql.CharacterStore) int32 {
	t.Helper()
	got, err := chars.ListByAccount(context.Background(), "player1")
	if err != nil {
		t.Fatalf("list characters: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("characters = %d, want 1", len(got))
	}
	return got[0].ID
}

func hasShortcut(shortcuts []shortcut.Shortcut, want shortcut.Shortcut) bool {
	for _, got := range shortcuts {
		if got == want {
			return true
		}
	}
	return false
}
