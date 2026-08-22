package lifecycle

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// spawnX/Y/Z is the class template's single spawn point: dropped items land
// inside the entering client's known range and stay clickable after a
// restart.
const (
	spawnX = 10
	spawnY = 20
	spawnZ = 30
)

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeEnterWorld() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes()
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

func encodeRequestItemList() []byte {
	return encodeSingleOpcode(clientpackets.OpcodeRequestItemList)
}

func encodeUseItem(objectID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	return w.Bytes()
}

func encodeRequestUnEquipItem(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}

func encodeRequestDropItem(objectID, count, x, y, z int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	return w.Bytes()
}

func encodeRequestDestroyItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeAction(objectID int32, x, y, z int32, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func assertFrameOpcode(t *testing.T, frame []byte, want byte, what string) {
	t.Helper()
	if frame[0] != want {
		t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
	}
}

// equipNoiseOpcodes are the grade/expertise-penalty packets an equip or
// unequip of graded gear may legitimately emit ahead of its UserInfo.
var equipNoiseOpcodes = map[byte]bool{
	serverpackets.OpcodeSkillList:       true,
	serverpackets.OpcodeEtcStatusUpdate: true,
}

// readSkippingEquipNoise reads frames until one whose opcode is not in
// equipNoiseOpcodes, returning it.
func readSkippingEquipNoise(t *testing.T, c *testsupport.ScriptedClient, what string) []byte {
	t.Helper()
	for i := 0; i < 10; i++ {
		frame := c.Read()
		if !equipNoiseOpcodes[frame[0]] {
			return frame
		}
	}
	t.Fatalf("no %s frame after skipping equip noise", what)
	return nil
}

func readEnterWorldBurst(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	want := []byte{
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeActionFailed,
	}
	frames := make([][]byte, 0, len(want))
	for i, opcode := range want {
		frame := c.Read()
		// A client that already knows another player receives that player's
		// CharInfo ahead of its own burst, and a client entering sight of a
		// restored ground item receives its SpawnItem; skip such leading
		// spawn frames.
		for i == 0 && (frame[0] == serverpackets.OpcodeCharInfo || frame[0] == serverpackets.OpcodeSpawnItem) {
			frame = c.Read()
		}
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		frames = append(frames, frame)
	}
	return frames
}

// drainUntilQuiet consumes every frame the client receives until the server
// stays quiet for a full read timeout.
func drainUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if c.ReadWithTimeout(300*time.Millisecond) == nil {
			return
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
}

// startInWorld selects slot 0 and enters the world, returning the EnterWorld
// burst frames and draining every trailing frame so callers read their own
// flow's frames from a quiet stream.
func startInWorld(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	drainUntilQuiet(t, c)
	return frames
}

// burstFrame returns the one frame in frames whose opcode is opcode.
func burstFrame(t *testing.T, frames [][]byte, opcode byte) []byte {
	t.Helper()
	for _, frame := range frames {
		if frame[0] == opcode {
			return frame
		}
	}
	t.Fatalf("no frame with opcode %#x in the EnterWorld burst", opcode)
	return nil
}

// itemListEntry is one row inside ItemList.
type itemListEntry struct {
	category uint16
	objID    int32
	itemID   int32
	count    int32
	equipped uint16
	enchant  uint16
	mana     int32
}

func readItemListEntries(t *testing.T, frame []byte) []itemListEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeItemList, "ItemList")
	r := wire.NewReader(frame[1:])
	r.ReadUint16() // showWindow
	n := r.ReadUint16()
	entries := make([]itemListEntry, 0, n)
	for i := uint16(0); i < n; i++ {
		var e itemListEntry
		e.category = r.ReadUint16()
		e.objID = r.ReadInt32()
		e.itemID = r.ReadInt32()
		e.count = r.ReadInt32()
		r.ReadUint16() // subCategory
		r.ReadUint16() // CustomType1
		e.equipped = r.ReadUint16()
		r.ReadInt32() // paperdoll slot
		e.enchant = r.ReadUint16()
		r.ReadUint16() // CustomType2
		r.ReadInt32()  // augmentation
		e.mana = r.ReadInt32()
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read ItemList: %v", err)
	}
	return entries
}

// findItemListEntry returns the ItemList row about objectID, or nil.
func findItemListEntry(entries []itemListEntry, objectID int32) *itemListEntry {
	for i := range entries {
		if entries[i].objID == objectID {
			return &entries[i]
		}
	}
	return nil
}

// inventoryEntry is one update row inside InventoryUpdate.
type inventoryEntry struct {
	state    uint16 // 1 added, 2 modified, 3 removed (per ItemState ordinal)
	objID    int32
	itemID   int32
	count    int32
	equipped uint16
	enchant  uint16
	mana     int32
}

func readInventoryUpdateEntries(t *testing.T, frame []byte) []inventoryEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")
	r := wire.NewReader(frame[1:])
	n := r.ReadUint16()
	entries := make([]inventoryEntry, 0, n)
	for i := uint16(0); i < n; i++ {
		var e inventoryEntry
		e.state = r.ReadUint16()
		r.ReadUint16() // item category
		e.objID = r.ReadInt32()
		e.itemID = r.ReadInt32()
		e.count = r.ReadInt32()
		r.ReadUint16() // subCategory
		r.ReadUint16() // CustomType1
		e.equipped = r.ReadUint16()
		r.ReadInt32() // paperdoll slot
		e.enchant = r.ReadUint16()
		r.ReadUint16() // CustomType2
		r.ReadInt32()  // augmentation
		e.mana = r.ReadInt32()
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read InventoryUpdate: %v", err)
	}
	return entries
}

// readInventoryUpdateFor asserts the next frame is an InventoryUpdate whose
// single entry reports objectID with wantCount.
func readInventoryUpdateFor(t *testing.T, c *testsupport.ScriptedClient, objectID, wantCount int32) inventoryEntry {
	t.Helper()
	entries := readInventoryUpdateEntries(t, c.Read())
	if len(entries) != 1 {
		t.Fatalf("InventoryUpdate entries = %+v, want exactly one", entries)
	}
	e := entries[0]
	if e.objID != objectID {
		t.Fatalf("InventoryUpdate object id = %d, want %d", e.objID, objectID)
	}
	if e.count != wantCount {
		t.Fatalf("InventoryUpdate count = %d, want %d", e.count, wantCount)
	}
	return e
}

// readDropItemGroundID asserts frame is the DropItem broadcast for a drop by
// dropperID of templateID and returns the fresh ground object id.
func readDropItemGroundID(t *testing.T, frame []byte, dropperID, templateID, count int32) int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != dropperID {
		t.Fatalf("DropItem dropper id = %d, want %d", got, dropperID)
	}
	groundID := r.ReadInt32()
	if groundID == dropperID {
		t.Fatalf("DropItem ground object id reused dropper id %d", dropperID)
	}
	if got := r.ReadInt32(); got != templateID {
		t.Fatalf("DropItem item id = %d, want %d", got, templateID)
	}
	x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if x != spawnX || y != spawnY || z != spawnZ {
		t.Fatalf("DropItem location = (%d,%d,%d), want (%d,%d,%d)", x, y, z, spawnX, spawnY, spawnZ)
	}
	if stackable := r.ReadInt32(); stackable != 1 {
		t.Fatalf("DropItem stackable = %d, want 1", stackable)
	}
	if got := r.ReadInt32(); got != count {
		t.Fatalf("DropItem count = %d, want %d", got, count)
	}
	return groundID
}
