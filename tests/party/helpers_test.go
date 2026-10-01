package party

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const quiet = 300 * time.Millisecond

// player is one dialed client and its character's object id.
type player struct {
	c    *testsupport.ScriptedClient
	id   int32
	name string
}

// group is a booted server with clients whose characters share the class
// template's spawn point, so every one of them sees the others.
type group struct {
	srv     *gameservertest.Server
	players []player
}

type seat struct {
	name  string
	level int
}

// bootGroup boots a server and dials one client per seat; the first seat
// is the boot's own character. Every client is brought into the world.
func bootGroup(t *testing.T, seats []seat, opts ...gameservertest.Option) *group {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter(seats[0].name, seats[0].level, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	g := &group{srv: srv, players: []player{{c: srv.Client, id: srv.SoleObjectID(t), name: seats[0].name}}}
	for i, s := range seats[1:] {
		account := "member" + string(rune('a'+i))
		id := srv.SeedCharacterFor(t, account, s.name, s.level, 0).ID
		g.players = append(g.players, player{c: srv.DialClient(t, account, 1), id: id, name: s.name})
	}
	for _, p := range g.players {
		startInWorld(t, p.c)
	}
	g.quiet(t)
	return g
}

// quiet drains every client.
func (g *group) quiet(t *testing.T) {
	t.Helper()
	for _, p := range g.players {
		drainFrames(t, p.c)
	}
}

// invite has from invite to into a party and has to accept, leaving every
// client drained.
func (g *group) invite(t *testing.T, from, to int, loot int32) {
	t.Helper()
	g.players[from].c.Send(encodeJoinParty(g.players[to].name, loot))
	assertSystemMessageText(t, skipPositions(g.players[from].c), serverpackets.SystemMessageYouInvitedS1ToParty, g.players[to].name)
	assertFrameOpcode(t, g.players[to].c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")
	g.players[to].c.Send(encodeAnswerJoinParty(1))
	g.quiet(t)
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

func encodeSingle(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

func encodeJoinParty(name string, loot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(loot)
	return w.Bytes()
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeName(opcode byte, name string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(name)
	return w.Bytes()
}

func encodeExtendedName(second uint16, name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(second)
	w.WriteString(name)
	return w.Bytes()
}

func encodeExtendedInt(second uint16, v int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(second)
	w.WriteInt32(v)
	return w.Bytes()
}

func startInWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	c.Send(encodeSingle(clientpackets.OpcodeEnterWorld))
	drainFrames(t, c)
}

// drainFrames collects every frame the client receives until the server
// goes quiet, in arrival order.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 200 {
		frame := c.ReadWithTimeout(quiet)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 200 drains")
	return nil
}

// assertSilent asserts c receives nothing but the party's periodic
// position reports.
func assertSilent(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	for {
		frame := c.ReadWithTimeout(quiet)
		if frame == nil {
			return
		}
		if frame[0] != serverpackets.OpcodePartyMemberPosition {
			t.Fatalf("%s: received %#x, want silence", what, frame[0])
		}
	}
}

func assertFrameOpcode(t *testing.T, frame []byte, want byte, what string) {
	t.Helper()
	if frame[0] != want {
		t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
	}
}

func assertStaticSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("system message %d params = %d, want 0", messageID, params)
	}
}

func assertSystemMessageText(t *testing.T, frame []byte, messageID int, text string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("system message %d params = %d, want 1", messageID, params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("system message %d param type = %d, want text", messageID, typ)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("system message %d text = %q, want %q", messageID, got, text)
	}
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}

// assertOpcodes asserts frames' opcodes are exactly want.
func assertOpcodes(t *testing.T, frames [][]byte, want []byte, what string) {
	t.Helper()
	if got := opcodes(frames); !bytes.Equal(got, want) {
		t.Fatalf("%s opcodes = %x, want %x", what, got, want)
	}
}

// windowRow is one member row of a party window packet.
type windowRow struct {
	objectID int32
	name     string
	level    int32
}

// readWindowAll decodes a PartySmallWindowAll.
func readWindowAll(t *testing.T, frame []byte) (leader, loot int32, rows []windowRow) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePartySmallWindowAll, "PartySmallWindowAll")
	r := wire.NewReader(frame[1:])
	leader, loot = r.ReadInt32(), r.ReadInt32()
	n := r.ReadInt32()
	for range n {
		row := windowRow{objectID: r.ReadInt32(), name: r.ReadString()}
		for range 6 {
			r.ReadInt32()
		}
		row.level = r.ReadInt32()
		r.ReadInt32() // class
		r.ReadInt32() // always 0
		r.ReadInt32() // race
		rows = append(rows, row)
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("PartySmallWindowAll: err %v, %d bytes left", err, r.Remaining())
	}
	return leader, loot, rows
}

// readWindowAdd decodes a PartySmallWindowAdd.
func readWindowAdd(t *testing.T, frame []byte) (leader, loot int32, row windowRow) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePartySmallWindowAdd, "PartySmallWindowAdd")
	r := wire.NewReader(frame[1:])
	leader, loot = r.ReadInt32(), r.ReadInt32()
	row = windowRow{objectID: r.ReadInt32(), name: r.ReadString()}
	for range 6 {
		r.ReadInt32()
	}
	row.level = r.ReadInt32()
	return leader, loot, row
}

// readWindowDelete decodes a PartySmallWindowDelete.
func readWindowDelete(t *testing.T, frame []byte) (int32, string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePartySmallWindowDelete, "PartySmallWindowDelete")
	r := wire.NewReader(frame[1:])
	return r.ReadInt32(), r.ReadString()
}

// skipPositions reads c's next frame that is not a PartyMemberPosition.
func skipPositions(c *testsupport.ScriptedClient) []byte {
	for {
		if frame := c.Read(); frame[0] != serverpackets.OpcodePartyMemberPosition {
			return frame
		}
	}
}
