package duel

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const quiet = 300 * time.Millisecond

// player is one dialed client and its character.
type player struct {
	c    *testsupport.ScriptedClient
	id   int32
	name string
}

// arena is a booted server whose characters share the class template's
// spawn point, so every one of them sees the others.
type arena struct {
	srv     *gameservertest.Server
	players []player
}

// bootArena boots a server and dials one level-20 character per name, the
// first one the boot's own, and brings every one of them into the world.
func bootArena(t *testing.T, names ...string) *arena {
	t.Helper()
	return bootArenaWith(t, nil, nil, names...)
}

// bootArenaWith is bootArena on a server booted with opts as well; before
// runs once every character is seeded, before any enters the world.
func bootArenaWith(t *testing.T, opts []gameservertest.Option, before func(*arena), names ...string) *arena {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter(names[0], 20, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	a := &arena{srv: srv, players: []player{{c: srv.Client, id: srv.SoleObjectID(t), name: names[0]}}}
	for i, name := range names[1:] {
		account := "duellist" + string(rune('a'+i))
		id := srv.SeedCharacterFor(t, account, name, 20, 0).ID
		a.players = append(a.players, player{c: srv.DialClient(t, account, 1), id: id, name: name})
	}
	if before != nil {
		before(a)
	}
	for _, p := range a.players {
		startInWorld(t, p.c)
	}
	a.quiet(t)
	return a
}

// quiet drains every client.
func (a *arena) quiet(t *testing.T) {
	t.Helper()
	for _, p := range a.players {
		drainFrames(t, p.c)
	}
}

// standing is a live player's duel standing.
type standing interface {
	InDuel() bool
	DuelID() int32
	DuelState() duel.State
	DuelTeam() int
}

func (a *arena) standing(t *testing.T, i int) standing {
	t.Helper()
	obj, ok := a.srv.State.Player(a.players[i].id)
	if !ok {
		t.Fatalf("player %d not in world", a.players[i].id)
	}
	return obj.(standing)
}

// vitals is a live player's HP surface.
type vitals interface {
	SetHP(float64)
	SetCP(float64)
	SetRollSource(func(int) int)
}

func (a *arena) vitals(t *testing.T, i int) vitals {
	t.Helper()
	obj, ok := a.srv.State.Player(a.players[i].id)
	if !ok {
		t.Fatalf("player %d not in world", a.players[i].id)
	}
	return obj.(vitals)
}

// onQueue runs fn on player i's queue and waits for it.
func (a *arena) onQueue(t *testing.T, i int, fn func()) {
	t.Helper()
	onQueue(t, a.srv.PlayerQueue(t, a.players[i].id), fn)
}

func onQueue(t *testing.T, q *sim.Queue, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !q.Post(func() { fn(); close(done) }) {
		t.Fatal("post: queue closed")
	}
	<-done
}

// challenge has from challenge to to a duel and to accept it, and returns
// once both fight, every client drained up to then.
func (a *arena) challenge(t *testing.T, from, to int) {
	t.Helper()
	a.players[from].c.Send(encodeDuelStart(a.players[to].name, false))
	a.quiet(t)
	a.players[to].c.Send(encodeDuelAnswer(false, true))
	a.srv.AdvanceUntil(t, "duel starts", func() bool {
		return a.standing(t, from).DuelState() == duel.Duelling && a.standing(t, to).DuelState() == duel.Duelling
	})
	a.quiet(t)
}

// party has from invite to into a party and to accept.
func (a *arena) party(t *testing.T, from, to int) {
	t.Helper()
	a.players[from].c.Send(encodeJoinParty(a.players[to].name))
	a.quiet(t)
	a.players[to].c.Send(encodeAnswerJoinParty(1))
	a.quiet(t)
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

func encodeDuelStart(name string, party bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelStart)
	w.WriteString(name)
	w.WriteInt32(wire.BoolInt32(party))
	return w.Bytes()
}

func encodeDuelAnswer(party, accept bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelAnswerStart)
	w.WriteInt32(wire.BoolInt32(party))
	w.WriteInt32(0)
	w.WriteInt32(wire.BoolInt32(accept))
	return w.Bytes()
}

func encodeDuelSurrender() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelSurrender)
	return w.Bytes()
}

func encodeAction(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeJoinParty(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeChangePartyLeader(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestChangePartyLeader)
	w.WriteString(name)
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
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	drainFrames(t, c)
}

// drainFrames collects every frame the client receives until the server
// goes quiet, in arrival order.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 400 {
		frame := c.ReadWithTimeout(quiet)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 400 drains")
	return nil
}

// systemMessage is one SystemMessage: its id and its text and number
// parameters, in order.
type systemMessage struct {
	id     int32
	params []any
}

func readSystemMessage(t *testing.T, frame []byte) systemMessage {
	t.Helper()
	r := wire.NewReader(frame[1:])
	m := systemMessage{id: r.ReadInt32()}
	n := r.ReadInt32()
	for range n {
		if r.ReadInt32() == serverpackets.SystemMessageParamText {
			m.params = append(m.params, r.ReadString())
		} else {
			m.params = append(m.params, r.ReadInt32())
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("SystemMessage: %v", err)
	}
	return m
}

// messages returns the system messages among frames, in order.
func messages(t *testing.T, frames [][]byte) []systemMessage {
	t.Helper()
	var out []systemMessage
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			out = append(out, readSystemMessage(t, f))
		}
	}
	return out
}

// indexOfMessage is the index in frames of the first system message id
// at or after from, or -1.
func indexOfMessage(t *testing.T, frames [][]byte, from int, id int) int {
	t.Helper()
	for i := from; i < len(frames); i++ {
		if frames[i][0] == serverpackets.OpcodeSystemMessage && readSystemMessage(t, frames[i]).id == int32(id) {
			return i
		}
	}
	return -1
}

// requireMessage fails unless frames carry system message id with params,
// at or after from, and returns the index after it.
func requireMessage(t *testing.T, frames [][]byte, from int, id int, params ...any) int {
	t.Helper()
	i := indexOfMessage(t, frames, from, id)
	if i < 0 {
		t.Fatalf("system message %d missing from %v", id, messages(t, frames))
	}
	got := readSystemMessage(t, frames[i]).params
	if len(got) != len(params) {
		t.Fatalf("system message %d params = %v, want %v", id, got, params)
	}
	for k := range params {
		if got[k] != params[k] {
			t.Fatalf("system message %d params = %v, want %v", id, got, params)
		}
	}
	return i + 1
}

// indexOfExtended is the index in frames of the first extended packet sub
// at or after from, or -1.
func indexOfExtended(frames [][]byte, from int, sub uint16) int {
	for i := from; i < len(frames); i++ {
		f := frames[i]
		if f[0] == serverpackets.OpcodeExtended && len(f) >= 3 && wire.NewReader(f[1:3]).ReadUint16() == sub {
			return i
		}
	}
	return -1
}

// requireExtended fails unless frames carry extended packet sub at or
// after from, and returns its index.
func requireExtended(t *testing.T, frames [][]byte, from int, sub uint16, what string) int {
	t.Helper()
	i := indexOfExtended(frames, from, sub)
	if i < 0 {
		t.Fatalf("%s missing", what)
	}
	return i
}

// indexOf is the index in frames of the first frame with opcode op at or
// after from, or -1.
func indexOf(frames [][]byte, from int, op byte) int {
	for i := from; i < len(frames); i++ {
		if frames[i][0] == op {
			return i
		}
	}
	return -1
}
