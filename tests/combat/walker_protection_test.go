package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// encodeBotMove is a MoveBackwardToLocation the way a bot client sends it:
// the six coordinates, then tail in place of the movement mode.
func encodeBotMove(tail ...byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int32{100, 200, -300, 10, 20, 30} {
		w.WriteInt32(v)
	}
	w.WriteBytes(tail)
	return w.Bytes()
}

// TestWalkerProtectionClosesMoveWithoutMovementMode sends, with
// L2WalkerProtection on, a movement request whose movement mode is missing
// or cut short: the session ends with ServerClose, not LeaveWorld, and the
// player leaves as on any server-ended session, the login server seeing
// the account log out.
func TestWalkerProtectionClosesMoveWithoutMovementMode(t *testing.T) {
	t.Parallel()
	for name, tail := range map[string][]byte{"missing": nil, "cut short": {1, 0}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1),
				gameservertest.WithChat(network.ChatConfig{WalkerProtection: true}),
			)
			c := srv.Client
			startInWorld(t, c)

			c.Send(encodeBotMove(tail...))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeServerClose, "bot movement answer")
			c.ExpectClosed()
			srv.AwaitLoginLogout(t, "player1")
		})
	}
}

// TestMoveWithoutMovementModeUnprotected sends the same requests with
// L2WalkerProtection off: each is refused with ActionFailed and the session
// stays.
func TestMoveWithoutMovementModeUnprotected(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)

	for _, tail := range [][]byte{nil, {1, 0}} {
		c.Send(encodeBotMove(tail...))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "unprotected bot movement answer")
		c.ExpectNoFrame()
	}
}
