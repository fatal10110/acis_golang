package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func readEnterWorldBurst(t *testing.T, c *testsupport.ScriptedClient, wantDie bool) [][]byte {
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
	}
	if wantDie {
		want = append(want, serverpackets.OpcodeDie)
	}
	want = append(want, serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)

	frames := make([][]byte, 0, len(want))
	for i, opcode := range want {
		frame := c.Read()
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		if i == 0 {
			want := []byte{serverpackets.OpcodeSendMacroList, 0, 0, 0, 0, 0, 0, 0}
			if !bytes.Equal(frame, want) {
				t.Fatalf("EnterWorld SendMacroList = %x, want %x", frame, want)
			}
		}
		if i == 1 {
			if second := wire.NewReader(frame[1:]).ReadUint16(); second != serverpackets.OpcodeExStorageMaxCount {
				t.Fatalf("EnterWorld first extended opcode = %#x, want ExStorageMaxCount (%#x)", second, serverpackets.OpcodeExStorageMaxCount)
			}
		}
		frames = append(frames, frame)
	}
	return frames
}
