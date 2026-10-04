package lifecycle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestStopSendsServerCloseToPlayerInWorld stops the server with a player in
// game: like the reference's shutdown, the client reads ServerClose as its
// last frame and then the close, so it leaves the world instead of hanging
// on a dead connection.
func TestStopSendsServerCloseToPlayerInWorld(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)

	srv.Close()

	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "stop ServerClose")
	srv.Client.ExpectClosed()
}

// TestStopSendsServerCloseToSelectedCharacter covers a character that is
// selected but has not sent EnterWorld: it is already registered in the
// world, so it is closed the same way.
func TestStopSendsServerCloseToSelectedCharacter(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	srv.Client.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")

	srv.Close()

	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "stop ServerClose")
	srv.Client.ExpectClosed()
}

// TestStopClosesCharacterListWithoutServerClose stops the server while the
// client is still on the character list: no character of it is in the world,
// so the connection just closes, as the reference's network shutdown closes
// it.
func TestStopClosesCharacterListWithoutServerClose(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))

	srv.Close()

	srv.Client.ExpectClosed()
}
