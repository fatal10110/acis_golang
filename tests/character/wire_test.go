package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeRequestCharacterCreate(name string, race, sex, classID int32, hairStyle, hairColor, face byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	w.WriteInt32(race)
	w.WriteInt32(sex)
	w.WriteInt32(classID)
	for i := 0; i < 6; i++ {
		w.WriteInt32(0)
	}
	w.WriteInt32(int32(hairStyle))
	w.WriteInt32(int32(hairColor))
	w.WriteInt32(int32(face))
	return w.Bytes()
}

func encodeRequestCharacterDelete(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterDelete)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeCharacterRestore(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCharacterRestore)
	w.WriteInt32(slot)
	return w.Bytes()
}

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

func encodeRequestManorList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
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

func encodeRequestChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

func encodeMoveBackwardToLocation(target, origin location.Location, moveMovement int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(int32(target.X))
	w.WriteInt32(int32(target.Y))
	w.WriteInt32(int32(target.Z))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteInt32(moveMovement)
	return w.Bytes()
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
		// CharInfo ahead of its own burst; skip those leading spawn frames.
		for i == 0 && frame[0] == serverpackets.OpcodeCharInfo {
			frame = c.Read()
		}
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		frames = append(frames, frame)
	}
	return frames
}

func assertStatusAttrs(t *testing.T, frame []byte, objectID int32, attrs []serverpackets.StatusAttribute) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("StatusUpdate opcode = %#x, want %#x", frame[0], serverpackets.OpcodeStatusUpdate)
	}
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

func waitForWorldPosition(t *testing.T, state *world.State, objID int32, want location.Location) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		obj, ok := state.Player(objID)
		if !ok {
			t.Fatal("world player missing while waiting for walk arrival")
		}
		positioned, ok := obj.(interface{ Position() (int, int, int) })
		if !ok {
			t.Fatal("world player has no Position method")
		}
		x, y, z := positioned.Position()
		if x == want.X && y == want.Y && z == want.Z {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("player position after walk = (%d,%d,%d), want %+v", x, y, z, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
