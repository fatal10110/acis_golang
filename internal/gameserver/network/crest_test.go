package network

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkRequestPledgeCrestDispatch(t *testing.T) {
	data := bytes.Repeat([]byte{0x5a}, 256)
	c, _, _, _ := newLinkedGameClientWithCrests(t, testCrestCache(t, map[int][]byte{101: data}))

	c.send(encodeRequestPledgeCrest(101))

	reply := c.read()
	assertPledgeCrestFrame(t, reply, 101, data)
}

func TestGameClientLinkRequestPledgeCrestDispatchMissingData(t *testing.T) {
	c, _, _, _ := newLinkedGameClientWithCrests(t, datacache.NewCrests())

	c.send(encodeRequestPledgeCrest(999))

	reply := c.read()
	assertPledgeCrestFrame(t, reply, 999, nil)
}

func TestAllowedGatesRequestPledgeCrest(t *testing.T) {
	if !Allowed(StateAuthed, clientpackets.OpcodeRequestPledgeCrest) {
		t.Fatal("Allowed(authed, RequestPledgeCrest) = false, want true")
	}
	if !Allowed(StateInGame, clientpackets.OpcodeRequestPledgeCrest) {
		t.Fatal("Allowed(in-game, RequestPledgeCrest) = false, want true")
	}
	if Allowed(StateConnected, clientpackets.OpcodeRequestPledgeCrest) {
		t.Fatal("Allowed(connected, RequestPledgeCrest) = true, want false")
	}
}

func encodeRequestPledgeCrest(crestID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeCrest)
	w.WriteInt32(crestID)
	return w.Bytes()
}

func testCrestCache(t *testing.T, crests map[int][]byte) *datacache.Crests {
	t.Helper()
	dir := t.TempDir()
	for id, data := range crests {
		path := filepath.Join(dir, "Crest_"+strconv.Itoa(id)+".dds")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write crest fixture: %v", err)
		}
	}
	cache, err := datacache.LoadCrests(dir)
	if err != nil {
		t.Fatalf("LoadCrests: %v", err)
	}
	return cache
}

func assertPledgeCrestFrame(t *testing.T, frame []byte, crestID int32, data []byte) {
	t.Helper()
	if frame[0] != serverpackets.OpcodePledgeCrest {
		t.Fatalf("PledgeCrest opcode = %#x, want %#x", frame[0], serverpackets.OpcodePledgeCrest)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != crestID {
		t.Fatalf("PledgeCrest crest id = %d, want %d", got, crestID)
	}
	size := r.ReadInt32()
	if size != int32(len(data)) {
		t.Fatalf("PledgeCrest data length = %d, want %d", size, len(data))
	}
	got := r.ReadBytes(int(size))
	if err := r.Err(); err != nil {
		t.Fatalf("read PledgeCrest: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("PledgeCrest data changed")
	}
}
