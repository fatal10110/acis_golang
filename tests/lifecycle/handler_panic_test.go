package lifecycle

import (
	"encoding/binary"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// panicOnSend makes the next frame with opcode that any connection of srv
// queues panic on the goroutine queueing it, as a handler bug would: the
// handler stops part-way, its reply never sent.
func panicOnSend(t *testing.T, srv *gameservertest.Server, opcode byte) {
	t.Helper()
	var armed atomic.Bool
	armed.Store(true)
	srv.ObserveSends(t, func(payload []byte) {
		if len(payload) > 0 && payload[0] == opcode && armed.CompareAndSwap(true, false) {
			panic("injected handler panic")
		}
	})
}

// TestPanicInPreAuthHandlerClosesConnection panics the ProtocolVersion
// handler on the read goroutine: the panic is recovered and the
// unauthenticated connection closes without a frame, as any refused
// handshake does. A player in the world is served on as before.
func TestPanicInPreAuthHandlerClosesConnection(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Bystander", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)
	panicOnSend(t, srv, serverpackets.OpcodeVersionCheck)

	probe := testsupport.Dial(t, srv.Addr())
	sendProtocolVersion(t, probe.Conn(), nil)
	probe.ExpectClosed()

	srv.Client.Send(encodeRequestItemList())
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeItemList, "item list after the recovered panic")
}

// TestPanicInCharacterSelectHandlerEndsSession panics the AuthLogin handler
// on the read goroutine as it answers the character list: the client reads
// ServerClose as its last frame, then the close; the teardown releases the
// account, so the login server no longer lists it, and the account logs in
// again normally.
func TestPanicInCharacterSelectHandlerEndsSession(t *testing.T) {
	t.Parallel()
	const account = "player2"
	srv := gameservertest.Boot(t)
	panicOnSend(t, srv, serverpackets.OpcodeCharSelectInfo)

	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	srv.IssueLoginSession(account, key)
	c := srv.DialAuthLogin(t, account, key)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeServerClose, "after the panicked AuthLogin")
	c.ExpectClosed()
	srv.AwaitLoginLogout(t, account)

	again := link.SessionKey{LoginKey1: 55, LoginKey2: 66, PlayKey1: 77, PlayKey2: 88}
	srv.IssueLoginSession(account, again)
	relog := srv.DialAuthLogin(t, account, again)
	assertFrameOpcode(t, relog.Read(), serverpackets.OpcodeCharSelectInfo, "relog CharSelectInfo")
	if !srv.LoginAccountOnline(t, account) {
		t.Fatal("login server does not list the account after it logged in again")
	}
}

// encodeMoveInPlace is a MoveBackwardToLocation with movement mode 0, which
// the read goroutine answers with ActionFailed alone.
func encodeMoveInPlace() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int32{spawnX + 50, spawnY, spawnZ, spawnX, spawnY, spawnZ, 0} {
		w.WriteInt32(v)
	}
	return w.Bytes()
}

// TestPanicInInWorldHandlerDetachesPlayer panics an in-world handler on the
// read goroutine as it answers a movement request: the client reads
// ServerClose as its last frame, then the close; the player leaves the
// world, so a bystander sees it deleted, and the account's logout is
// reported.
func TestPanicInInWorldHandlerDetachesPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Panicker", 5, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	srv.SeedCharacterFor(t, "bystander", "Bystander", 1, 0)
	bystander := srv.DialClient(t, "bystander", 1)
	startInWorld(t, srv.Client)
	startInWorld(t, bystander)
	drainUntilQuiet(t, srv.Client)
	panicOnSend(t, srv, serverpackets.OpcodeActionFailed)

	srv.Client.Send(encodeMoveInPlace())
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "after the panicked movement handler")
	srv.Client.ExpectClosed()
	deleted := readUntilOpcode(t, bystander, serverpackets.OpcodeDeleteObject)
	if got := int32(binary.LittleEndian.Uint32(deleted[1:5])); got != objID {
		t.Fatalf("bystander DeleteObject id = %d, want the panicked player %d", got, objID)
	}
	srv.AwaitLoginLogout(t, srv.Account())
	if _, inWorld := srv.State.Player(objID); inWorld {
		t.Fatal("player still in the world after its handler panicked")
	}
}
