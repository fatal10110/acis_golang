package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// readItemRegisterGroup reads the ShortCutRegister answering an item
// registration and returns the object id and shared reuse group it carries.
func readItemRegisterGroup(t *testing.T, reply []byte) (objectID, group int32) {
	t.Helper()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}
	r := wire.NewReader(reply[1:])
	if typ := r.ReadInt32(); typ != int32(serverpackets.ShortcutItem) {
		t.Fatalf("ShortCutRegister type = %d, want item", typ)
	}
	r.ReadInt32() // slot
	objectID = r.ReadInt32()
	r.ReadInt32() // character type
	group = r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("parse ShortCutRegister: %v", err)
	}
	return objectID, group
}

// TestShortcutRegisterCarriesEtcItemReuseGroup binds a potion to the bar in
// the session it is held: the ShortCutRegister answer already carries the
// potion's shared reuse group, so the slot shows the group's cooldown
// without a relog, and the kept entry is saved. A binding for an object the
// player does not hold is answered with no group.
func TestShortcutRegisterCarriesEtcItemReuseGroup(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Binder", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, lesserHealingPotionID, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainQuiet(t, c)

	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), wireShortcutSlot(0, 1), potion, 1))
	if id, group := readItemRegisterGroup(t, c.Read()); id != potion || group != lesserPotionReuseGroup {
		t.Fatalf("potion ShortCutRegister = object %d group %d, want object %d group %d", id, group, potion, lesserPotionReuseGroup)
	}

	const missingObjectID int32 = 998
	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), wireShortcutSlot(0, 2), missingObjectID, 1))
	if id, group := readItemRegisterGroup(t, c.Read()); id != missingObjectID || group != -1 {
		t.Fatalf("missing-object ShortCutRegister = object %d group %d, want object %d group -1", id, group, missingObjectID)
	}

	srv.FlushPersistence(t)
	rows, err := srv.Shortcuts.ListByOwner(context.Background(), objID, 0)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	var saved int
	for _, row := range rows {
		if row.Type == shortcut.Item && row.ID == potion && row.Page == 0 && row.Slot == 1 {
			saved++
		}
	}
	if saved != 1 || len(rows) != 1 {
		t.Fatalf("stored shortcuts = %+v, want the potion's alone at page 0 slot 1", rows)
	}
}
