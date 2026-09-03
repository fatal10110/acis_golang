package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
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

func encodeAttackRequest(objectID int32, x, y, z int32, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestUnEquipItem(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}

func encodeRequestDropItem(objectID, count int32, x, y, z int32) []byte {
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

func encodeRequestCrystallizeItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCrystallizeItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeRequestEnchantItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestEnchantItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeRequestAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeAnswerTradeRequest(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAnswerTradeRequest)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestPackageSendableItemList(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPackageItemList)
	w.WriteInt32(objectID)
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

func encodeMoveBackwardToLocation(targetX, targetY, targetZ, originX, originY, originZ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(targetX)
	w.WriteInt32(targetY)
	w.WriteInt32(targetZ)
	w.WriteInt32(originX)
	w.WriteInt32(originY)
	w.WriteInt32(originZ)
	w.WriteInt32(1)
	return w.Bytes()
}

// startInWorld selects slot 0 and enters the world, consuming the fixed
// EnterWorld reply burst plus every trailing frame (level-grant SkillList,
// late ActionFailed) so callers read their own flow's frames from a quiet
// stream.
func startInWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainUntilQuiet(t, c)
}

// equipNoiseOpcodes are the grade/expertise-penalty packets an equip or
// unequip of graded gear may legitimately emit ahead of its UserInfo; item
// suites skip them instead of pinning penalty behavior covered elsewhere.
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
		serverpackets.OpcodeSendMacroList,
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
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
		// CharInfo ahead of its own burst, and a client carrying weighted
		// items receives the login weight refresh ahead of it; skip such
		// leading spawn frames.
		for i == 0 && (frame[0] == serverpackets.OpcodeCharInfo || frame[0] == serverpackets.OpcodeStatusUpdate) {
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

func assertFrameOpcode(t *testing.T, frame []byte, want byte, what string) {
	t.Helper()
	if frame[0] != want {
		t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
	}
}

// inventoryEntry is one update row inside InventoryUpdate.
type inventoryEntry struct {
	state    uint16 // 1 added, 2 modified, 3 removed (per ItemState ordinal)
	objID    int32
	itemID   int32
	count    int32
	equipped uint16
	enchant  uint16
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
		r.ReadInt32()  // mana left
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

// findInventoryUpdate scans frames for an InventoryUpdate entry about
// objectID, failing if none carries it.
func findInventoryUpdate(t *testing.T, frames [][]byte, objectID int32) inventoryEntry {
	t.Helper()
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readInventoryUpdateEntries(t, frame) {
			if e.objID == objectID {
				return e
			}
		}
	}
	t.Fatalf("no InventoryUpdate entry for object %d in %d frames", objectID, len(frames))
	return inventoryEntry{}
}

// assertStaticSystemMessage asserts frame is a parameterless SystemMessage
// with the given message id.
func assertStaticSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	if len(frame) != 9 {
		t.Fatalf("SystemMessage frame = %x, want a static 9-byte frame", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("system message params = %d, want 0", got)
	}
}

// systemMessageID returns the message id of a SystemMessage frame without
// asserting its parameter shape.
func systemMessageID(t *testing.T, frame []byte) int {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	return int(wire.NewReader(frame[1:]).ReadInt32())
}

// assertSystemMessageItem asserts a SystemMessage with one item-name param.
func assertSystemMessageItem(t *testing.T, frame []byte, messageID int, itemID int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("param count = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
		t.Fatalf("param type = %d, want item name", typ)
	}
	if got := r.ReadInt32(); got != itemID {
		t.Fatalf("item id = %d, want %d", got, itemID)
	}
}

// assertExAutoSoulShot asserts an ExAutoSoulShot(itemID, enabled) packet.
func assertExAutoSoulShot(t *testing.T, frame []byte, itemID int32, enabled bool) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "extended")
	r := wire.NewReader(frame[1:])
	if second := r.ReadUint16(); second != serverpackets.OpcodeExAutoSoulShot {
		t.Fatalf("extended opcode = %#x, want ExAutoSoulShot (%#x)", second, serverpackets.OpcodeExAutoSoulShot)
	}
	if got := r.ReadInt32(); got != itemID {
		t.Fatalf("item id = %d, want %d", got, itemID)
	}
	wantEnabled := int32(0)
	if enabled {
		wantEnabled = 1
	}
	if got := r.ReadInt32(); got != wantEnabled {
		t.Fatalf("enabled = %d, want %d", got, wantEnabled)
	}
}

// assertMagicSkillUseSelf asserts a MagicSkillUse cast by and on objectID.
func assertMagicSkillUseSelf(t *testing.T, frame []byte, objectID, skillID, level, hitTime, reuse int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	r := wire.NewReader(frame[1:])
	caster, target, sid, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != objectID || target != objectID || sid != skillID || lvl != level {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d level %d, want %d/%d/%d/%d",
			caster, target, sid, lvl, objectID, objectID, skillID, level)
	}
	gotHit, gotReuse := r.ReadInt32(), r.ReadInt32()
	if gotHit != hitTime || gotReuse != reuse {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want %d/%d", gotHit, gotReuse, hitTime, reuse)
	}
}

// statusUpdateAttrs parses a StatusUpdate frame into its type/value pairs.
func statusUpdateAttrs(t *testing.T, frame []byte) map[serverpackets.StatusType]int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "StatusUpdate")
	r := wire.NewReader(frame[1:])
	objID := r.ReadInt32()
	n := r.ReadInt32()
	attrs := make(map[serverpackets.StatusType]int32, n)
	for i := int32(0); i < n; i++ {
		attrs[serverpackets.StatusType(r.ReadInt32())] = r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate for %d: %v", objID, err)
	}
	return attrs
}
