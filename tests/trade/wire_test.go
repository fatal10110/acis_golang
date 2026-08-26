package trade

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// spawnX/Y/Z is the class template's single spawn point every client of a
// boot shares, so both traders start well inside the 150-unit interaction
// radius.
const (
	spawnX = 10
	spawnY = 20
	spawnZ = 30

	tradeInteractionDistance = 150
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

func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeAnswerTradeRequest(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAnswerTradeRequest)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeAddTradeItem(tradeID, objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAddTradeItem)
	w.WriteInt32(tradeID)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeTradeDone(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeDone)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeMoveBackwardToLocation(targetX, targetY, targetZ, originX, originY, originZ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(targetX)
	w.WriteInt32(targetY)
	w.WriteInt32(targetZ)
	w.WriteInt32(originX)
	w.WriteInt32(originY)
	w.WriteInt32(originZ)
	w.WriteInt32(1)
	return w.Bytes()
}

// traders is a booted server with two dialed clients whose characters are
// seeded but not yet in the world; tests give items before calling enterAll
// so the inventories load them.
type traders struct {
	srv      *gameservertest.Server
	first    *testsupport.ScriptedClient
	firstID  int32
	second   *testsupport.ScriptedClient
	secondID int32
}

func bootTraders(t *testing.T) *traders {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("TraderOne", 1, 0),
		gameservertest.WithWantChars(1),
	)
	h := &traders{
		srv:      srv,
		first:    srv.Client,
		firstID:  srv.SoleObjectID(t),
		secondID: srv.SeedCharacterFor(t, "player2", "TraderTwo", 1, 0).ID,
	}
	h.second = srv.DialClient(t, "player2", 1)
	return h
}

// third dials an extra client on its own account so a scenario can occupy
// the target with a pending request.
func (h *traders) third(t *testing.T, account, name string) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	id := h.srv.SeedCharacterFor(t, account, name, 1, 0).ID
	c := h.srv.DialClient(t, account, 1)
	startInWorld(t, c)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
	drainUntilQuiet(t, c)
	return c, id
}

// enterAll brings both traders into the world and drains the mutual
// spawn/known noise so callers read their own flow's frames.
func (h *traders) enterAll(t *testing.T) {
	t.Helper()
	startInWorld(t, h.first)
	startInWorld(t, h.second)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
}

// startTrade performs the request/answer handshake and consumes its packets,
// leaving both clients inside an open trade window.
func (h *traders) startTrade(t *testing.T) {
	t.Helper()
	h.first.Send(encodeTradeRequest(h.secondID))
	frame := h.second.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != h.firstID {
		t.Fatalf("SendTradeRequest requester id = %d, want %d", got, h.firstID)
	}
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")

	h.second.Send(encodeAnswerTradeRequest(1))
	for _, who := range []struct {
		name    string
		client  *testsupport.ScriptedClient
		partner string
	}{
		{"first", h.first, "TraderTwo"},
		{"second", h.second, "TraderOne"},
	} {
		assertSystemMessageText(t, who.client.Read(), serverpackets.SystemMessageBeginTradeWithS1, who.partner)
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeTradeStart, who.name+" TradeStart")
		r := wire.NewReader(frame[1:])
		wantPartner := h.secondID
		if who.name == "second" {
			wantPartner = h.firstID
		}
		if got := r.ReadInt32(); got != wantPartner {
			t.Fatalf("%s TradeStart partner id = %d, want %d", who.name, got, wantPartner)
		}
	}
}

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
		// CharInfo ahead of its own burst; skip such leading spawn frames.
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

// waitForArrival polls the live player's world position until the walk to
// wantX/spawnY/spawnZ registers, failing after a deadline.
func waitForArrival(t *testing.T, h *traders, objID, wantX int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		obj, ok := h.srv.State.Player(objID)
		if !ok {
			t.Fatalf("world.Player(%d) missing while waiting for walk arrival", objID)
		}
		positioned, ok := obj.(interface{ Position() (int, int, int) })
		if !ok {
			t.Fatalf("world.Player(%d) = %T has no Position method", objID, obj)
		}
		x, _, _ := positioned.Position()
		if int32(x) == wantX {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("player %d position x = %d after walk, want %d", objID, x, wantX)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func drainUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if c.ReadWithTimeout(300*time.Millisecond) == nil {
			return
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
}

// drainFrames collects every frame the client receives until the server goes
// quiet, returning them in arrival order.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 8)
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return nil
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
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("system message params = %d, want 0", params)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

// assertSystemMessageText asserts a SystemMessage with one text param.
func assertSystemMessageText(t *testing.T, frame []byte, messageID int, text string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", id, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("param count = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("param type = %d, want text", typ)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("system message text = %q, want %q", got, text)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}
