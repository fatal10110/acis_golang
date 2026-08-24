package combat

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

func encodeLogout() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes()
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
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

// encodeAttackRequest mirrors the client's AttackRequest: the same
// target-and-origin shape an Action click carries.
func encodeAttackRequest(objectID int32, x, y, z int32, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestTargetCancel(unselect int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestTargetCancel)
	w.WriteInt32(unselect)
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

func encodeUseItem(objectID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeRequestRestartPoint(requestType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRestartPoint)
	w.WriteInt32(requestType)
	return w.Bytes()
}

// startInWorld selects slot 0 and enters the world, consuming the fixed
// EnterWorld reply burst plus every trailing frame so callers read their own
// flow's frames from a quiet stream.
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

// readEnterWorldBurst consumes the fixed EnterWorld reply burst in wire
// order, skipping other players' spawn broadcasts (CharInfo) the server can
// interleave whenever someone is already in the shared spawn region.
func readEnterWorldBurst(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	want := []byte{
		serverpackets.OpcodeSendMacroList,
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
	for i, opcode := range want {
		var frame []byte
		for {
			frame = c.Read()
			if frame == nil {
				t.Fatalf("EnterWorld frame %d (want %#x) never arrived", i, opcode)
			}
			if frame[0] != serverpackets.OpcodeCharInfo {
				break
			}
		}
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
	}
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

// readUntil collects frames until one matches want, returning every frame
// read including the match.
func readUntil(t *testing.T, c *testsupport.ScriptedClient, want byte, what string) [][]byte {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatalf("%s never arrived", what)
		}
		if frame[0] == want {
			return [][]byte{frame}
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
	return nil
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s not observed within 5s", what)
}

func assertFrameOpcode(t *testing.T, frame []byte, want byte, what string) {
	t.Helper()
	if frame[0] != want {
		t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
	}
}

func wireReader(payload []byte) *wire.Reader {
	return wire.NewReader(payload)
}

// assertSystemMessageSkillFrame asserts a SystemMessage carrying one
// skill-name parameter.
func assertSystemMessageSkillFrame(t *testing.T, frame []byte, messageID int, skillID, level int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wireReader(frame[1:])
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

// assertSystemMessageNumber asserts a SystemMessage whose only parameter is
// one number.
func assertSystemMessageNumber(t *testing.T, frame []byte, messageID int, values ...int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wireReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != int32(len(values)) {
		t.Fatalf("SystemMessage params = %d, want %d", params, len(values))
	}
	for _, value := range values {
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
			t.Fatalf("SystemMessage param type = %d, want number", typ)
		}
		if got := r.ReadInt32(); got != value {
			t.Fatalf("SystemMessage param value = %d, want %d", got, value)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

// assertSystemMessageStringFrame asserts a SystemMessage carrying a single
// text parameter with the given contents.
func assertSystemMessageStringFrame(t *testing.T, frame []byte, messageID int, text string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wireReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("SystemMessage id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("SystemMessage params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("SystemMessage param type = %d, want string", typ)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("SystemMessage text = %q, want %q", got, text)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}
