package network

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

func TestGameClientLinkRequestAllyCrestDispatch(t *testing.T) {
	data := bytes.Repeat([]byte{0x7b}, 192)
	c := newInGameClientWithCrests(t, testAllyCrestCache(t, map[int][]byte{103: data}))

	c.send(encodeRequestAllyCrest(103))

	reply := c.read()
	assertAllyCrestFrame(t, reply, 103, data)
}

func TestGameClientLinkRequestAllyCrestDispatchMissingData(t *testing.T) {
	c := newInGameClientWithCrests(t, datacache.NewCrests())

	c.send(encodeRequestAllyCrest(999))

	assertNoReply(t, c)
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

func TestAllowedGatesRequestAllyCrest(t *testing.T) {
	if Allowed(StateAuthed, clientpackets.OpcodeRequestAllyCrest) {
		t.Fatal("Allowed(authed, RequestAllyCrest) = true, want false")
	}
	if !Allowed(StateInGame, clientpackets.OpcodeRequestAllyCrest) {
		t.Fatal("Allowed(in-game, RequestAllyCrest) = false, want true")
	}
	if Allowed(StateConnected, clientpackets.OpcodeRequestAllyCrest) {
		t.Fatal("Allowed(connected, RequestAllyCrest) = true, want false")
	}
}

func encodeRequestPledgeCrest(crestID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeCrest)
	w.WriteInt32(crestID)
	return w.Bytes()
}

func encodeRequestAllyCrest(crestID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAllyCrest)
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

func testAllyCrestCache(t *testing.T, crests map[int][]byte) *datacache.Crests {
	t.Helper()
	dir := t.TempDir()
	for id, data := range crests {
		path := filepath.Join(dir, "AllyCrest_"+strconv.Itoa(id)+".dds")
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

func newInGameClientWithCrests(t *testing.T, crests *datacache.Crests) *fakeGameClient {
	t.Helper()
	c, _, _, _, _ := newLinkedGameClientWithSkillsShortcutsCrestsSeed(t, nil, nil, crests, modelskill.BookPolicy{}, nil, func(chars *fakeCharStore, items *fakeItemStore) {
		seedSelectableCharacter(t, chars, "player1", "CrestTester", 1, 0)
	}, 1)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	return c
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

func assertAllyCrestFrame(t *testing.T, frame []byte, crestID int32, data []byte) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeAllyCrest {
		t.Fatalf("AllyCrest opcode = %#x, want %#x", frame[0], serverpackets.OpcodeAllyCrest)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != crestID {
		t.Fatalf("AllyCrest crest id = %d, want %d", got, crestID)
	}
	size := r.ReadInt32()
	if size != int32(len(data)) {
		t.Fatalf("AllyCrest data length = %d, want %d", size, len(data))
	}
	got := r.ReadBytes(int(size))
	if err := r.Err(); err != nil {
		t.Fatalf("read AllyCrest: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("AllyCrest data changed")
	}
}

func assertNoReply(t *testing.T, c *fakeGameClient) {
	t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if frame, err := wire.ReadFrame(c.conn); err == nil {
		t.Fatalf("unexpected frame: %x", frame)
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("ReadFrame error = %v, want timeout", err)
	}
}
