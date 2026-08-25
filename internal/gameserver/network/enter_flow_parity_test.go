package network

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The tests here pin the enter-flow parity fixes: the CONNECTED-state
// dispatch width, silent RequestGameStart refusals, duplicate-login
// eviction, the ENTERING quest-list probe, the dropped SkillCoolTime
// request reply, and char-delete's list refresh on an unknown slot.

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

// rawClient drives frames before and after the cipher handshake without
// ScriptedClient's handshake-first restriction, for the pre-auth dispatch
// ordering test.
type rawClient struct {
	t      *testing.T
	conn   net.Conn
	cipher *gamecipher.Cipher
}

func dialRaw(t *testing.T, addr string) *rawClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return &rawClient{t: t, conn: conn}
}

func (r *rawClient) write(payload []byte) {
	r.t.Helper()
	buf := append([]byte(nil), payload...)
	if r.cipher != nil {
		r.cipher.Encrypt(buf)
	}
	if err := wire.WriteFrame(r.conn, buf); err != nil {
		r.t.Fatalf("write frame: %v", err)
	}
}

func (r *rawClient) read() []byte {
	r.t.Helper()
	r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(r.conn)
	if err != nil {
		r.t.Fatalf("read frame: %v", err)
	}
	if r.cipher != nil {
		r.cipher.Decrypt(payload)
	}
	return payload
}

func (r *rawClient) expectNoFrame() {
	r.t.Helper()
	r.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if payload, err := wire.ReadFrame(r.conn); err == nil {
		if r.cipher != nil {
			r.cipher.Decrypt(payload)
		}
		r.t.Fatalf("unexpected frame: %x", payload)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		r.t.Fatalf("read frame: %v", err)
	}
}

// TestAuthLoginAcceptedBeforeProtocolVersion pins the CONNECTED-state
// dispatch width: both 0x00 (SendProtocolVersion) and 0x08 (AuthLogin) are
// accepted unconditionally before any version exchange, so an AuthLogin
// arriving first is answered with CharSelectInfo instead of a disconnect,
// and the connection still completes the handshake afterwards.
func TestAuthLoginAcceptedBeforeProtocolVersion(t *testing.T) {
	addr, chars, _ := twoClientServer(t)
	seedSelectableCharacter(t, chars, "player1", "Newbie", 1, 0)

	c := dialRaw(t, addr)
	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString("player1")
	w.WriteInt32(key.PlayKey2)
	w.WriteInt32(key.PlayKey1)
	w.WriteInt32(key.LoginKey1)
	w.WriteInt32(key.LoginKey2)
	c.write(w.Bytes())

	if reply := c.read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}

	// Once authed, the reference's AUTHED state no longer dispatches 0x00:
	// prove the connection survived by sending an AUTHED-state request.
	c.write(encodeCharacterRestore(9))
	if frame := c.read(); frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want NewCharacterSuccess (%#x)", frame[0], serverpackets.OpcodeNewCharacterSuccess)
	}
}

// TestRequestGameStartUnknownSlotKeepsConnectionOpen pins that an unknown
// character-select slot aborts the selection silently — no SSQInfo, no
// CharSelected — and leaves the connection usable for further requests.
func TestRequestGameStartUnknownSlotKeepsConnectionOpen(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestGameStart(5))
	c.ExpectNoFrame()

	// The connection still dispatches: a restore attempt answers with the
	// refreshed character list.
	c.Send(encodeCharacterRestore(9))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-refusal opcode = %#x, want CharSelectInfo (%#x)", frame[0], serverpackets.OpcodeCharSelectInfo)
	}
}

// TestRequestGameStartBannedCharacterRefusedSilently pins that a character
// whose AccessLevel is negative cannot be selected: the reference refuses
// the selection without any reply, leaving the connection on the character
// list.
func TestRequestGameStartBannedCharacterRefusedSilently(t *testing.T) {
	c, _, _, _ := newLinkedGameClientWithSkillsSeed(t, nil, func(chars *fakeCharStore, _ *fakeItemStore) {
		seedSelectableCharacter(t, chars, "player1", "Banned", 1, 0)
		list, err := chars.ListByAccount(context.Background(), "player1")
		if err != nil || len(list) != 1 {
			t.Fatalf("list seeded character: %v (%d)", err, len(list))
		}
		// Banned before the client ever authenticates, so no server
		// goroutine can race the mutation.
		list[0].AccessLevel = -100
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.ExpectNoFrame()

	// The connection still dispatches: a restore attempt answers with the
	// refreshed character list.
	c.Send(encodeCharacterRestore(9))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-refusal opcode = %#x, want CharSelectInfo (%#x)", frame[0], serverpackets.OpcodeCharSelectInfo)
	}
}

// TestDuplicateCharacterLoginClosesPreviousClientAndAbortsNewSelection
// pins the existing-player branch of character loading: selecting a
// character that already belongs to a live session closes that session's
// client (ServerClose then close) and aborts the new selection silently.
func TestDuplicateCharacterLoginClosesPreviousClientAndAbortsNewSelection(t *testing.T) {
	c, chars, _, state := newLinkedGameClientSeedOneChar(t)
	list, err := chars.ListByAccount(context.Background(), "player1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list seeded character: %v (%d)", err, len(list))
	}
	objectID := list[0].ObjectID()

	// Register the character as already in-world, owned by another live
	// session whose connection we can observe closing.
	previous := newTestLivePlayer(t, objectID, &testsupport.FrameCapture{})
	kicked := make(chan struct{}, 1)
	previous.kick = func() { kicked <- struct{}{} }
	state.AddPlayer(previous)

	c.Send(encodeRequestGameStart(0))

	select {
	case <-kicked:
	case <-time.After(2 * time.Second):
		t.Fatal("previous session was not kicked")
	}
	c.ExpectNoFrame()

	// The aborted selection leaves the connection alive: an unknown-slot
	// delete answers with the refreshed character list.
	c.Send(encodeRequestCharacterDelete(9))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-abort opcode = %#x, want CharSelectInfo (%#x)", frame[0], serverpackets.OpcodeCharSelectInfo)
	}
}

// TestQuestListProbeDuringEnteringAnswersEmptyQuestList pins the 0x3f probe
// the client sends while loading: it must be answered with QuestList during
// ENTERING even though no skill list or live player exists yet.
func TestQuestListProbeDuringEnteringAnswersEmptyQuestList(t *testing.T) {
	c, _, _, _ := newLinkedGameClientSeedOneChar(t)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	frame := c.Read()
	if frame[0] != serverpackets.OpcodeQuestList {
		t.Fatalf("probe reply opcode = %#x, want QuestList (%#x)", frame[0], serverpackets.OpcodeQuestList)
	}
	if len(frame) != 3 || frame[1] != 0 || frame[2] != 0 {
		t.Fatalf("QuestList payload = %x, want empty list", frame)
	}
}

// TestKnownExtendedOpcodeWhileEnteringCountsTowardDisconnect pins that a
// known IN_GAME extended sub-opcode sent during ENTERING is not absorbed by
// its in-game handler's live-player guard but counts toward the
// unknown-packet disconnect threshold like any other unhandled packet.
func TestKnownExtendedOpcodeWhileEnteringCountsTowardDisconnect(t *testing.T) {
	c, _, _, _ := newLinkedGameClientSeedOneChar(t)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected

	for i := 0; i < maxUnknownPerMin; i++ {
		c.Send(encodeRequestAutoSoulShot(11, 1))
	}
	c.ExpectClosed()
}

// TestRequestSkillCoolTimeInGameGetsNoReply pins that the in-game 0x9d
// request stays unanswered — reuse timers reach the client unsolicited —
// while the connection itself remains alive.
func TestRequestSkillCoolTimeInGameGetsNoReply(t *testing.T) {
	c, _, _, _, _ := newLinkedGameClientEnterWorld(t)

	c.Send(encodeRequestSkillCoolTime())
	c.ExpectNoFrame()

	// The connection is still dispatching: manor list answers immediately.
	c.Send(encodeRequestManorList())
	if frame := c.Read(); frame[0] != serverpackets.OpcodeExtended {
		t.Fatalf("post-drop opcode = %#x, want ExSendManorList under Extended (%#x)", frame[0], serverpackets.OpcodeExtended)
	}
}

// TestRequestCharacterDeleteUnknownSlotRefreshesListWithoutFailPacket pins
// that deleting an unknown slot sends no CharDeleteFail — only the
// refreshed CharSelectInfo the reference resends unconditionally after
// every delete attempt.
func TestRequestCharacterDeleteUnknownSlotRefreshesListWithoutFailPacket(t *testing.T) {
	c, _, _, _ := newLinkedGameClientSeedOneChar(t)

	c.Send(encodeRequestCharacterDelete(4))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("unknown-slot delete opcode = %#x, want CharSelectInfo (%#x)", frame[0], serverpackets.OpcodeCharSelectInfo)
	}
}

// --- helpers ---

func newLinkedGameClientSeedOneChar(t *testing.T) (*testsupport.ScriptedClient, *fakeCharStore, *fakeItemStore, *world.State) {
	t.Helper()
	c, chars, items, _, state := newLinkedGameClientWithSkillsShortcutsSeed(t, nil, nil, func(chars *fakeCharStore, _ *fakeItemStore) {
		seedSelectableCharacter(t, chars, "player1", "Newbie", 1, 0)
	}, 1)
	return c, chars, items, state
}

func newLinkedGameClientEnterWorld(t *testing.T) (*testsupport.ScriptedClient, *fakeCharStore, *fakeItemStore, *fakeShortcutStore, *world.State) {
	t.Helper()
	c, chars, items, state := newLinkedGameClientSeedOneChar(t)
	shortcuts := (*fakeShortcutStore)(nil)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	return c, chars, items, shortcuts, state
}

// twoClientServer wires one game-client listener behind a fake login
// server, returning its address so several independent clients can connect
// to the same world state.
func twoClientServer(t *testing.T) (addr string, chars *fakeCharStore, state *world.State) {
	t.Helper()
	loginAddr, servers, sessions := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	validator := NewSessionValidator()
	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	loginLink, err := DialLoginLink(context.Background(), loginAddr, auth, LoginLinkHandlers{PlayerAuthResponse: validator.Resolve}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	t.Cleanup(func() { loginLink.Close() })

	addr, chars, _, _, state = newTestGameClientLinkWithSkillsShortcutsAndLog(t, func() *LoginLink { return loginLink }, validator, nil, zerolog.Nop())
	sessions.Put("player1", link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44})
	return addr, chars, state
}

// TestSessionCloseSendsServerCloseThenCloses pins the eviction wire order:
// ServerClose goes out first, and the connection is closed once it has been
// written.
func TestSessionCloseSendsServerCloseThenCloses(t *testing.T) {
	client, server := net.Pipe()
	conn := newConn(server, zerolog.Nop())
	t.Cleanup(func() { client.Close() })
	cipher, err := gamecipher.NewCipher(make([]byte, gamecipher.KeySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	session := NewSession(conn, cipher)

	done := make(chan struct{})
	go func() {
		session.Close()
		close(done)
	}()

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	frame, err := wire.ReadFrame(client)
	if err != nil {
		t.Fatalf("read ServerClose: %v", err)
	}
	if frame[0] != serverpackets.OpcodeServerClose {
		t.Fatalf("opcode = %#x, want ServerClose (%#x)", frame[0], serverpackets.OpcodeServerClose)
	}

	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection still open after ServerClose")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Session.Close did not return")
	}
}
