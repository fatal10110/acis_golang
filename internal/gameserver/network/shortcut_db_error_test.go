package network

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Regression coverage for #1649: ShortcutList.addShortcut/deleteShortcut
// (ShortcutList.java:62-116) update in-memory state and send the client
// echo unconditionally; the DB write is wrapped in its own try/catch that
// only logs on failure. restore() (ShortcutList.java:173-209) is likewise
// wrapped in a catch-all that logs and returns, so a shortcut-load failure
// never aborts login. A DB hiccup must not turn into a dropped client packet
// or a failed login.

func TestGameClientLinkRegistersShortcutDespiteSaveError(t *testing.T) {
	c, chars, _, shortcuts, _ := newLinkedGameClientWithShortcuts(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	shortcuts.mu.Lock()
	shortcuts.saveErr = errors.New("forced save error")
	shortcuts.mu.Unlock()

	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutAction), 15, 5, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("opcode = %#x, want ShortCutRegister (%#x) even though the DB save failed", reply[0], serverpackets.OpcodeShortCutRegister)
	}

	for _, sc := range shortcuts.shortcuts(objID) {
		if sc.Slot == 3 && sc.Page == 1 && sc.ID == 5 {
			t.Fatalf("new shortcut persisted despite forced save error: %+v", sc)
		}
	}
}

func TestGameClientLinkDeletesShortcutDespiteDeleteError(t *testing.T) {
	c, chars, _, shortcuts, _ := newLinkedGameClientWithShortcuts(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	shortcuts.seed(objID, shortcut.Shortcut{Slot: 3, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1})
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	shortcuts.mu.Lock()
	shortcuts.deleteErr = errors.New("forced delete error")
	shortcuts.mu.Unlock()

	c.Send(encodeRequestShortCutDel(15))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeShortCutDelete {
		t.Fatalf("opcode = %#x, want ShortCutDelete (%#x) even though the DB delete failed", reply[0], serverpackets.OpcodeShortCutDelete)
	}
}

func TestGameClientLinkEnterWorldSurvivesShortcutListError(t *testing.T) {
	c, chars, _, shortcuts, _ := newLinkedGameClientWithShortcuts(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	chars.soleObjectID(t)

	shortcuts.mu.Lock()
	shortcuts.listByOwnerErr = errors.New("forced list error")
	shortcuts.mu.Unlock()

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())

	// Login must still succeed and reach the normal EnterWorld burst,
	// including an (empty) ShortCutInit, instead of dying on the shortcut
	// read failure.
	readEnterWorldBurst(t, c, false)
}
