package character

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// lesserHealingPotionID is the shared catalog's Lesser Healing Potion
// fixture (shared_reuse_group 8).
const lesserHealingPotionID = 1060

// lesserPotionReuseGroup is the fixture potion's shared reuse group.
const lesserPotionReuseGroup = 8

// wireShortcutSlot maps a persisted (page, slot) pair to the wire slot the
// client sends: slot = page*12 + slot.
func wireShortcutSlot(page, slot int32) int32 { return page*12 + slot }

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

// rejectSilenceWindow is how long a test waits to confirm a handler stayed
// silent.
const rejectSilenceWindow = 300 * time.Millisecond

// drainQuiet reads until the stream stays quiet for one silence window.
func drainQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for range 100 {
		if c.ReadWithTimeout(rejectSilenceWindow) == nil {
			return
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
}

// shortCutInitEntry is one parsed ShortCutInit row.
type shortCutInitEntry struct {
	typ           serverpackets.ShortcutType
	slot          int32
	id            int32
	characterType int32
	reuseGroup    int32
}

func parseShortCutInit(t *testing.T, frame []byte) []shortCutInitEntry {
	t.Helper()
	if frame[0] != serverpackets.OpcodeShortCutInit {
		t.Fatalf("opcode = %#x, want ShortCutInit (%#x)", frame[0], serverpackets.OpcodeShortCutInit)
	}
	r := wire.NewReader(frame[1:])
	entries := make([]shortCutInitEntry, 0, 8)
	for range r.ReadInt32() {
		var e shortCutInitEntry
		e.typ = serverpackets.ShortcutType(r.ReadInt32())
		e.slot = r.ReadInt32()
		e.id = r.ReadInt32()
		switch e.typ {
		case serverpackets.ShortcutSkill:
			r.ReadInt32() // level
			r.ReadUint8() // marker
			e.characterType = r.ReadInt32()
		case serverpackets.ShortcutItem:
			e.characterType = r.ReadInt32()
			e.reuseGroup = r.ReadInt32()
			r.ReadInt32() // remaining
			r.ReadInt32() // reuse
			r.ReadInt32() // augment
		default:
			e.characterType = r.ReadInt32()
		}
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("parse ShortCutInit: %v", err)
	}
	return entries
}

func findShortCut(entries []shortCutInitEntry, typ serverpackets.ShortcutType, id int32) *shortCutInitEntry {
	for i := range entries {
		if entries[i].typ == typ && entries[i].id == id {
			return &entries[i]
		}
	}
	return nil
}

// TestShortcutFlowRegistersPersistsDeletesDropsStale drives the shortcut
// bar over the real wire protocol: restore-time stale-row drop and
// shared-reuse-group population, action/item registration (with the silent
// rejection of an item objectId outside the inventory), deletion, and the
// surviving rows coming back on the next enter-world burst.
func TestShortcutFlowRegistersPersistsDeletesDropsStale(t *testing.T) {
	const staleObjectID int32 = 999
	const missingObjectID int32 = 998

	logs := &logBuffer{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithLog(zerolog.New(logs)),
		gameservertest.WithReuseDelays(0, 0),
	)
	t.Cleanup(func() {
		if t.Failed() {
			logs.mu.Lock()
			defer logs.mu.Unlock()
			t.Logf("server log:\n%s", logs.buf.String())
		}
	})
	c := srv.Client
	objID := srv.SoleObjectID(t)
	for _, sc := range []shortcut.Shortcut{
		{Slot: 2, Page: 1, Type: shortcut.Action, ID: 5, Level: -1, CharacterType: 1},
		{Slot: 4, Page: 1, Type: shortcut.Item, ID: staleObjectID, Level: -1, CharacterType: 1},
	} {
		if err := srv.Shortcuts.Save(context.Background(), objID, sc); err != nil {
			t.Fatalf("seed shortcut %+v: %v", sc, err)
		}
	}
	potion := srv.GiveItem(t, objID, lesserHealingPotionID, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected

	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	initEntries := parseShortCutInit(t, frames[11])
	if e := findShortCut(initEntries, serverpackets.ShortcutAction, 5); e == nil || e.slot != wireShortcutSlot(1, 2) {
		t.Fatalf("ShortCutInit action entry = %+v, want action id 5 at wire slot %d", e, wireShortcutSlot(1, 2))
	}
	if e := findShortCut(initEntries, serverpackets.ShortcutItem, staleObjectID); e != nil {
		t.Fatalf("ShortCutInit still includes stale item shortcut for missing object %d: %+v", staleObjectID, e)
	}
	drainQuiet(t, c)

	// Registering the owned potion persists an ITEM shortcut row.
	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), wireShortcutSlot(1, 3), potion, 1))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("item register opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}

	// A registration for an objectId outside the inventory is silently
	// dropped: no reply, no persisted row (ShortcutList.java ITEM branch).
	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), wireShortcutSlot(1, 5), missingObjectID, 1))
	if frame := c.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("registration for missing object answered %#x, want silence", frame[0])
	}

	rows, err := srv.Shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	var hasPotion, hasMissing bool
	for _, row := range rows {
		switch {
		case row.Type == shortcut.Item && row.ID == potion:
			hasPotion = true
		case row.Type == shortcut.Item && row.ID == missingObjectID:
			hasMissing = true
		}
	}
	if !hasPotion || hasMissing {
		t.Fatalf("persisted shortcuts = %+v, want potion row %d and no row for %d", rows, potion, missingObjectID)
	}

	// Deleting the seeded action shortcut answers ShortCutDelete and drops
	// the row.
	c.Send(encodeRequestShortCutDel(wireShortcutSlot(1, 2)))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeShortCutDelete {
		t.Fatalf("delete opcode = %#x, want ShortCutDelete (%#x)", reply[0], serverpackets.OpcodeShortCutDelete)
	}
	rows, err = srv.Shortcuts.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list shortcuts after delete: %v", err)
	}
	for _, row := range rows {
		if row.Type == shortcut.Action && row.ID == 5 {
			t.Fatalf("deleted action shortcut still persisted: %+v", rows)
		}
	}

	// Restart back to selection and re-enter: the surviving potion shortcut
	// comes back with its shared reuse group recomputed from the item data,
	// and neither the deleted nor the stale row reappears.
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("pre-restart opcode = %#x, want ActionFailed", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("restart opcode = %#x, want RestartResponse", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-restart opcode = %#x, want CharSelectInfo", reply[0])
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected

	c.Send(encodeEnterWorld())
	frames = readEnterWorldBurst(t, c)
	initEntries = parseShortCutInit(t, frames[11])
	potionEntry := findShortCut(initEntries, serverpackets.ShortcutItem, potion)
	if potionEntry == nil {
		t.Fatalf("ShortCutInit after restart missing potion shortcut %d: %+v", potion, initEntries)
	}
	if potionEntry.slot != wireShortcutSlot(1, 3) || potionEntry.characterType != 1 || potionEntry.reuseGroup != lesserPotionReuseGroup {
		t.Fatalf("restored potion shortcut = slot %d charType %d reuseGroup %d, want slot %d charType 1 reuseGroup %d",
			potionEntry.slot, potionEntry.characterType, potionEntry.reuseGroup, wireShortcutSlot(1, 3), lesserPotionReuseGroup)
	}
	if e := findShortCut(initEntries, serverpackets.ShortcutAction, 5); e != nil {
		t.Fatalf("deleted action shortcut came back on restart: %+v", e)
	}
	if e := findShortCut(initEntries, serverpackets.ShortcutItem, staleObjectID); e != nil {
		t.Fatalf("stale item shortcut came back on restart: %+v", e)
	}
}
