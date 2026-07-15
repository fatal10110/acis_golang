package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkEnterWorldSendsPersistedShortcuts(t *testing.T) {
	c, chars, _, shortcuts, _ := newLinkedGameClientWithShortcuts(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	shortcuts.seed(objID, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1})

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c, false)

	frame := frames[9]
	r := wire.NewReader(frame[1:])
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("ShortCutInit count = %d, want 1", count)
	}
	if typ, slot, id, characterType := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); typ != int32(serverpackets.ShortcutAction) || slot != 15 || id != 5 || characterType != 1 {
		t.Fatalf("ShortCutInit entry = type %d slot %d id %d charType %d, want action slot 15 id 5 charType 1", typ, slot, id, characterType)
	}
}

func TestGameClientLinkRegistersShortcut(t *testing.T) {
	c, chars, _, shortcuts, _ := newLinkedGameClientWithShortcuts(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestShortCutReg(int32(serverpackets.ShortcutAction), 15, 5, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}
	got := shortcuts.shortcuts(objID)
	want := shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1}
	if !hasShortcut(got, want) {
		t.Fatalf("shortcuts = %+v, want %+v", got, want)
	}
}

func TestGameClientLinkRegistersSkillShortcutAtKnownLevel(t *testing.T) {
	c, chars, _, shortcuts, _ := newLinkedGameClientWithShortcuts(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	chars.updateCharacter(t, objID, func(ch *player.Character) {
		ch.SetSkillLevel(248, 3)
	})
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestShortCutReg(int32(serverpackets.ShortcutSkill), 15, 248, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}
	r := wire.NewReader(reply[1:])
	if typ, slot, id, level, marker, characterType := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadUint8(), r.ReadInt32(); typ != int32(serverpackets.ShortcutSkill) || slot != 15 || id != 248 || level != 3 || marker != 0 || characterType != 1 {
		t.Fatalf("ShortCutRegister skill = type %d slot %d id %d level %d marker %d charType %d, want skill slot 15 id 248 level 3 marker 0 charType 1", typ, slot, id, level, marker, characterType)
	}
	got := shortcuts.shortcuts(objID)
	want := shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Skill, ID: 248, Level: 3, CharacterType: 1}
	if !hasShortcut(got, want) {
		t.Fatalf("shortcuts = %+v, want %+v", got, want)
	}
}

func TestGameClientLinkDeletesShortcut(t *testing.T) {
	c, chars, _, shortcuts, _ := newLinkedGameClientWithShortcuts(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	shortcuts.seed(objID, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1})
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestShortCutDel(15))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeShortCutDelete {
		t.Fatalf("opcode = %#x, want ShortCutDelete (%#x)", reply[0], serverpackets.OpcodeShortCutDelete)
	}
	if got := shortcuts.shortcuts(objID); len(got) != 0 {
		t.Fatalf("shortcuts after delete = %+v, want empty", got)
	}
}

func hasShortcut(shortcuts []shortcut.Shortcut, want shortcut.Shortcut) bool {
	for _, got := range shortcuts {
		if got == want {
			return true
		}
	}
	return false
}
