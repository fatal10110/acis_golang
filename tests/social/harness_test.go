package social

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const silenceWindow = 300 * time.Millisecond

// pair is a booted server with two dialed clients, Alice (the primary
// account) and Bobby, seeded but not yet in the world.
type pair struct {
	srv     *gameservertest.Server
	alice   *testsupport.ScriptedClient
	aliceID int32
	bobby   *testsupport.ScriptedClient
	bobbyID int32
}

func bootPair(t *testing.T, opts ...gameservertest.Option) *pair {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Alice", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	}, opts...)...)
	p := &pair{
		srv:     srv,
		alice:   srv.Client,
		aliceID: srv.SoleObjectID(t),
		bobbyID: srv.SeedCharacterFor(t, "player2", "Bobby", 1, 0).ID,
	}
	p.bobby = srv.DialClient(t, "player2", 1)
	return p
}

// enterAll brings both players into the world and drains the mutual
// spawn noise.
func (p *pair) enterAll(t *testing.T) {
	t.Helper()
	startInWorld(t, p.alice)
	startInWorld(t, p.bobby)
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, p.bobby)
}

// third dials and enters a third player on its own account.
func (p *pair) third(t *testing.T, account, name string) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	id := p.srv.SeedCharacterFor(t, account, name, 1, 0).ID
	c := p.srv.DialClient(t, account, 1)
	startInWorld(t, c)
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, p.bobby)
	drainUntilQuiet(t, c)
	return c, id
}

// awaitOffline lets time pass until objID has left the world.
func (p *pair) awaitOffline(t *testing.T, objID int32) {
	t.Helper()
	p.srv.AdvanceUntil(t, "player leaves the world", func() bool {
		_, ok := p.srv.State.Player(objID)
		return !ok
	})
}

// restart takes c's character back to character selection.
func (p *pair) restart(t *testing.T, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	assertOpcode(t, c.Read(), serverpackets.OpcodeRestartResponse, "RestartResponse")
	p.awaitOffline(t, objID)
	drainUntilQuiet(t, c)
}

func (p *pair) setAccessLevel(t *testing.T, objID int32, level int) {
	t.Helper()
	if _, err := p.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", level, objID); err != nil {
		t.Fatalf("set access level of %d: %v", objID, err)
	}
}

func startInWorld(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	assertOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	return readEnterWorldBurst(t, c)
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
		for i == 0 && (frame[0] == serverpackets.OpcodeCharInfo || frame[0] == serverpackets.OpcodeRelationChanged) {
			frame = c.Read()
		}
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		frames = append(frames, frame)
		if opcode == serverpackets.OpcodeEtcStatusUpdate {
			gameservertest.ReadInitialCompass(t, c, serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged)
		}
	}
	return frames
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

func encodeNamed(opcode byte, name string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(name)
	return w.Bytes()
}

func encodeFriendInvite(name string) []byte {
	return encodeNamed(clientpackets.OpcodeRequestFriendInvite, name)
}

func encodeFriendDel(name string) []byte {
	return encodeNamed(clientpackets.OpcodeRequestFriendDel, name)
}

func encodeAnswerFriendInvite(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerFriendInvite)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeFriendList() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestFriendList).Bytes()
}

func encodeBlock(typ int32, name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBlock)
	w.WriteInt32(typ)
	if typ == clientpackets.BlockAdd || typ == clientpackets.BlockRemove {
		w.WriteString(name)
	}
	return w.Bytes()
}

func encodeFriendSay(message, recipient string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSendL2FriendSay)
	w.WriteString(message)
	w.WriteString(recipient)
	return w.Bytes()
}

func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func drainUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for range 100 {
		if c.ReadWithTimeout(silenceWindow) == nil {
			return
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
}

// drainFrames collects every frame c receives until the server goes quiet.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.ReadWithTimeout(silenceWindow)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return nil
}

func assertSilent(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	if frame := c.ReadWithTimeout(silenceWindow); frame != nil {
		t.Fatalf("%s: received %#x, want silence", what, frame[0])
	}
}

func assertOpcode(t *testing.T, frame []byte, want byte, what string) {
	t.Helper()
	if frame[0] != want {
		t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
	}
}

// assertStaticSystemMessage asserts a parameterless SystemMessage.
func assertStaticSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("system message %d params = %d, want 0", messageID, params)
	}
}

// assertSystemMessageText asserts a SystemMessage with one text param.
func assertSystemMessageText(t *testing.T, frame []byte, messageID int, text string) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
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

// assertAddResult asserts a FriendAddRequestResult.
func assertAddResult(t *testing.T, frame []byte, accepted bool) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeFriendAddRequestResult, "FriendAddRequestResult")
	want := int32(0)
	if accepted {
		want = 1
	}
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != want {
		t.Fatalf("FriendAddRequestResult = %d, want %d", got, want)
	}
}

// assertL2Friend asserts an L2Friend row change.
func assertL2Friend(t *testing.T, frame []byte, action int32, name string, online bool, objectID int32) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeL2Friend, "L2Friend")
	r := wire.NewReader(frame[1:])
	gotAction, zero, gotName, gotOnline, gotID := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadInt32(), r.ReadInt32()
	wantOnline := int32(0)
	if online {
		wantOnline = 1
	}
	if gotAction != action || zero != 0 || gotName != name || gotOnline != wantOnline || gotID != objectID {
		t.Fatalf("L2Friend = (%d, %d, %q, %d, %d), want (%d, 0, %q, %d, %d)", gotAction, zero, gotName, gotOnline, gotID, action, name, wantOnline, objectID)
	}
}

// relationRow reads the character_relations flags of the pair a, b (lower
// id first), or -1 when there is no row.
func relationRow(t *testing.T, p *pair, a, b int32) int {
	t.Helper()
	if a > b {
		a, b = b, a
	}
	var rel int
	err := p.srv.DB.QueryRowContext(context.Background(), "SELECT relation FROM character_relations WHERE char_id = ? AND friend_id = ?", a, b).Scan(&rel)
	if err != nil {
		return -1
	}
	return rel
}
