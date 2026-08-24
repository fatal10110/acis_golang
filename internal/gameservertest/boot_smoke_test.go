package gameservertest

import (
	"context"
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(sqltest.Main(m))
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

// readEnterWorldBurst consumes and opcode-checks the fixed EnterWorld reply
// burst.
func readEnterWorldBurst(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
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

// TestBootFullFlow proves the harness end to end: seed a character through
// the SQL store, select and enter the world over the real wire protocol,
// observe the live player in world state, then give an item through the item
// store and see it persisted.
func TestBootFullFlow(t *testing.T) {
	srv := Boot(t, WithCharacter("Newbie", 1, 0), WithWantChars(1))

	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}

	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)

	objID := srv.SoleObjectID(t)
	live, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("world player missing after EnterWorld")
	}
	if live.ObjectID() != objID {
		t.Fatalf("live player object id = %d, want %d", live.ObjectID(), objID)
	}

	srv.GiveItem(t, objID, item.AdenaID, 1000)
	instances, err := srv.Items.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	var found bool
	for _, inst := range instances {
		if inst.TemplateID == item.AdenaID && inst.Count == 1000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("seeded adena missing from persisted items, got %d items", len(instances))
	}
}
