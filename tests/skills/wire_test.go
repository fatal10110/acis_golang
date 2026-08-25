package skills

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

func encodeLogout() []byte {
	return encodeSingleOpcode(clientpackets.OpcodeLogout)
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

func encodeMoveBackwardToLocation(targetX, targetY, targetZ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(targetX)
	w.WriteInt32(targetY)
	w.WriteInt32(targetZ)
	w.WriteInt32(10)
	w.WriteInt32(20)
	w.WriteInt32(30)
	w.WriteInt32(1)
	return w.Bytes()
}

func encodeRequestAcquireSkillInfo(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkillInfo)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestAcquireSkill(skillID, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkill)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestExMagicSkillUseGround(x, y, z, skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestExMagicSkillUseGround)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestSkillCoolTime() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestSkillCoolTime).Bytes()
}

func encodeRequestExEnchantSkill(skillID, level int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestExEnchantSkill)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	return w.Bytes()
}

// startInWorld selects slot 0 and enters the world, consuming the fixed
// EnterWorld reply burst plus every trailing frame so callers read their own
// flow's frames from a quiet stream.
func startInWorld(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	drainUntilQuiet(t, c)
	return frames
}

// startInWorldAmongPlayers selects slot 0 and enters the world while other
// players may already be present: their CharInfo spawn broadcasts can
// interleave anywhere in the selection and EnterWorld reply sequences, so
// those frames are skipped by opcode rather than consumed by position.
func startInWorldAmongPlayers(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	sawSSQ, sawSelected := false, false
	for !sawSSQ || !sawSelected {
		frame := c.Read()
		switch frame[0] {
		case serverpackets.OpcodeSSQInfo:
			sawSSQ = true
		case serverpackets.OpcodeCharSelected:
			sawSelected = true
		case serverpackets.OpcodeCharInfo:
		default:
			t.Fatalf("selection frame opcode %#x, want SSQInfo/CharSelected", frame[0])
		}
	}

	c.Send(encodeEnterWorld())
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
	for i := 0; i < len(want); {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeCharInfo {
			continue
		}
		if frame[0] != want[i] {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], want[i])
		}
		frames = append(frames, frame)
		i++
	}
	drainUntilQuiet(t, c)
	return frames
}

// readEnterWorldBurst consumes the fixed EnterWorld reply burst and returns
// its frames in wire order.
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
	return readFrameSequence(t, c, want)
}

// readEnterWorldBurstWithRestoredBuff consumes the EnterWorld burst for a
// character whose saved effects are replayed between HennaInfo and
// EtcStatusUpdate (the updateEffectIcons hook), so one AbnormalStatusUpdate
// frame lands there that a plain entry does not expect.
func readEnterWorldBurstWithRestoredBuff(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	want := []byte{
		serverpackets.OpcodeSendMacroList,
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeAbnormalStatusUpdate,
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
	return readFrameSequence(t, c, want)
}

func readFrameSequence(t *testing.T, c *testsupport.ScriptedClient, want []byte) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, len(want))
	for i, opcode := range want {
		frame := c.Read()
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

// assertStaticSystemMessage asserts frame is a parameterless SystemMessage
// with the given message id.
func assertStaticSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("system message params = %d, want 0", got)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

// assertSystemMessageSkillFrame asserts a SystemMessage carrying one
// skill-name parameter.
func assertSystemMessageSkillFrame(t *testing.T, frame []byte, messageID int, skillID, level int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("SystemMessage params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamSkillName {
		t.Fatalf("SystemMessage param type = %d, want skill name", typ)
	}
	if id := r.ReadInt32(); id != skillID {
		t.Fatalf("SystemMessage skill id = %d, want %d", id, skillID)
	}
	if got := r.ReadInt32(); got != level {
		t.Fatalf("SystemMessage skill level = %d, want %d", got, level)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

// assertSPStatus asserts a StatusUpdate reporting the character's SP.
func assertSPStatus(t *testing.T, frame []byte, objectID int32, sp int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "StatusUpdate")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objectID)
	}
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("StatusUpdate count = %d, want 1", count)
	}
	if typ := r.ReadInt32(); typ != int32(serverpackets.StatusSP) {
		t.Fatalf("StatusUpdate type = %d, want SP", typ)
	}
	if got := r.ReadInt32(); got != int32(sp) {
		t.Fatalf("StatusUpdate SP = %d, want %d", got, sp)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}

// assertStatusAttrs asserts a StatusUpdate carrying exactly the given
// attributes in order.
func assertStatusAttrs(t *testing.T, frame []byte, objectID int32, attrs []serverpackets.StatusAttribute) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "StatusUpdate")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objectID)
	}
	if count := r.ReadInt32(); count != int32(len(attrs)) {
		t.Fatalf("StatusUpdate count = %d, want %d", count, len(attrs))
	}
	for _, attr := range attrs {
		if typ := r.ReadInt32(); typ != int32(attr.Type) {
			t.Fatalf("StatusUpdate type = %d, want %d", typ, attr.Type)
		}
		if got := r.ReadInt32(); got != int32(attr.Value) {
			t.Fatalf("StatusUpdate value = %d, want %d", got, attr.Value)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}

// abnormalStatusEntry is one decoded AbnormalStatusUpdate icon entry.
type abnormalStatusEntry struct {
	SkillID  int32
	Level    uint16
	Duration int32
}

// readAbnormalStatusUpdateEntries asserts the next frame is an
// AbnormalStatusUpdate and returns its decoded icon entries in wire order.
func readAbnormalStatusUpdateEntries(t *testing.T, c *testsupport.ScriptedClient) []abnormalStatusEntry {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeAbnormalStatusUpdate, "AbnormalStatusUpdate")
	r := wire.NewReader(frame[1:])
	count := r.ReadUint16()
	entries := make([]abnormalStatusEntry, 0, count)
	for range count {
		entries = append(entries, abnormalStatusEntry{SkillID: r.ReadInt32(), Level: r.ReadUint16(), Duration: r.ReadInt32()})
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AbnormalStatusUpdate: %v", err)
	}
	return entries
}

// assertAbnormalStatusUpdate asserts the next frame is an
// AbnormalStatusUpdate whose single entry is the given skill.
func assertAbnormalStatusUpdate(t *testing.T, c *testsupport.ScriptedClient, skillID, level int32, minDuration int32) {
	t.Helper()
	entries := readAbnormalStatusUpdateEntries(t, c)
	if len(entries) != 1 {
		t.Fatalf("AbnormalStatusUpdate entries = %+v, want exactly the skill %d icon", entries, skillID)
	}
	e := entries[0]
	if e.SkillID != skillID || int32(e.Level) != level {
		t.Fatalf("AbnormalStatusUpdate entry = %d/%d, want %d/%d", e.SkillID, e.Level, skillID, level)
	}
	if minDuration > 0 && e.Duration < minDuration {
		t.Fatalf("AbnormalStatusUpdate duration = %d, want at least %d", e.Duration, minDuration)
	}
}

// coolTimeEntry is one decoded SkillCoolTime reuse row.
type coolTimeEntry struct {
	SkillID          int32
	Level            int32
	ReuseSeconds     int32
	RemainingSeconds int32
}

// readSkillCoolTimeEntries asserts the next frame is SkillCoolTime and
// returns its decoded rows in wire order.
func readSkillCoolTimeEntries(t *testing.T, c *testsupport.ScriptedClient) []coolTimeEntry {
	t.Helper()
	return readSkillCoolTimeEntriesFromFrame(t, c.Read())
}

func readSkillCoolTimeEntriesFromFrame(t *testing.T, frame []byte) []coolTimeEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSkillCoolTime, "SkillCoolTime")
	r := wire.NewReader(frame[1:])
	count := r.ReadInt32()
	entries := make([]coolTimeEntry, 0, count)
	for range count {
		entries = append(entries, coolTimeEntry{
			SkillID:          r.ReadInt32(),
			Level:            r.ReadInt32(),
			ReuseSeconds:     r.ReadInt32(),
			RemainingSeconds: r.ReadInt32(),
		})
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SkillCoolTime: %v", err)
	}
	return entries
}

// assertShortCutRegister asserts the next frame is ShortCutRegister pointing
// the given slot at the given skill id and level.
func assertShortCutRegister(t *testing.T, c *testsupport.ScriptedClient, slot, skillID, level int32) {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeShortCutRegister, "ShortCutRegister")
	r := wire.NewReader(frame[1:])
	typ, gotSlot, id, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if typ != int32(serverpackets.ShortcutSkill) || gotSlot != slot || id != skillID || lvl != level {
		t.Fatalf("ShortCutRegister = type %d slot %d id %d level %d, want skill slot %d id %d level %d",
			typ, gotSlot, id, lvl, slot, skillID, level)
	}
}

func encodeRequestExEnchantSkillInfo(skillID, level int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestExEnchantSkillInfo)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	return w.Bytes()
}

// readStatusUpdateSkippingAbnormal reads frames until the character's
// StatusUpdate arrives, skipping any AbnormalStatusUpdate refreshes the
// effect add broadcasts alongside it, and returns those icon entries.
func readStatusUpdateSkippingAbnormal(t *testing.T, c *testsupport.ScriptedClient, objectID int32, attrs []serverpackets.StatusAttribute) []abnormalStatusEntry {
	t.Helper()
	var icons []abnormalStatusEntry
	for i := 0; i < 6; i++ {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeAbnormalStatusUpdate {
			icons = append(icons, readAbnormalStatusUpdateEntriesFromFrame(t, frame)...)
			continue
		}
		assertStatusAttrs(t, frame, objectID, attrs)
		return icons
	}
	t.Fatal("no StatusUpdate after cast completion")
	return nil
}

func readAbnormalStatusUpdateEntriesFromFrame(t *testing.T, frame []byte) []abnormalStatusEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeAbnormalStatusUpdate, "AbnormalStatusUpdate")
	r := wireReader(frame[1:])
	count := r.ReadUint16()
	entries := make([]abnormalStatusEntry, 0, count)
	for range count {
		entries = append(entries, abnormalStatusEntry{SkillID: r.ReadInt32(), Level: r.ReadUint16(), Duration: r.ReadInt32()})
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AbnormalStatusUpdate: %v", err)
	}
	return entries
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s not observed within 3s", what)
}
