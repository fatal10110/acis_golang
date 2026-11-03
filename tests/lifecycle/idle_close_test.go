package lifecycle

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The idle deadline is not waited out: ExpireIdleReads makes the next wait
// for a frame time out at once, so each test sends one frame, reads its
// answer, and then the ServerClose the deadline makes. The driven clock
// counts a read loop as caught up once it waits for a frame, which the
// expired wait does before it ends the connection, so each test stops the
// server, which waits for the handler, before it checks the close.

// TestIdleCharacterListClosesWithServerClose leaves a client idle on the
// character list: it reads ServerClose as its last frame before the close,
// so it shows the disconnect instead of hanging on a dead connection.
func TestIdleCharacterListClosesWithServerClose(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	srv.ExpireIdleReads()

	srv.Client.Send(encodeRequestPledgeCrest())
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodePledgeCrest, "pledge crest")
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "idle ServerClose")
	srv.Stop()
	srv.Client.ExpectClosed()
}

// TestIdlePlayerInWorldClosesWithServerClose leaves a player idle in the
// world: the client reads ServerClose and the close, and the player then
// leaves the world as on a dropped connection, saved offline.
func TestIdlePlayerInWorldClosesWithServerClose(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)
	srv.ExpireIdleReads()

	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeSkillList, "skill list")
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "idle ServerClose")

	srv.AdvanceUntil(t, "idle player leaves the world", func() bool {
		_, ok := srv.State.PlayerByName("Newbie")
		return !ok
	})
	srv.Stop()
	srv.Client.ExpectClosed()
	var online int
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT online FROM characters WHERE char_name = ?", "Newbie").Scan(&online); err != nil {
		t.Fatalf("read online: %v", err)
	}
	if online != 0 {
		t.Fatalf("online = %d after the idle close, want 0", online)
	}
}

// TestFailedSelectionClosesWithServerClose ends an authenticated session from
// a handler (a selected character whose class has no template): the client
// reads ServerClose before the close instead of being dropped silently.
func TestFailedSelectionClosesWithServerClose(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	if _, err := srv.DB.ExecContext(context.Background(),
		"UPDATE characters SET classid = 250, base_class = 250 WHERE char_name = ?", "Newbie"); err != nil {
		t.Fatalf("set unknown class: %v", err)
	}

	srv.Client.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "failed selection ServerClose")
	srv.Client.ExpectClosed()
}

// TestEmptyFrameIsSkipped sends a frame with no body: it is skipped unread
// and the connection keeps serving the frames after it.
func TestEmptyFrameIsSkipped(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))

	srv.Client.Send(nil)
	srv.Client.Send(encodeRequestPledgeCrest())
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodePledgeCrest, "pledge crest after an empty frame")
}

// encodeRequestPledgeCrest asks for a crest no clan has: a request the
// character list answers with one frame.
func encodeRequestPledgeCrest() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeCrest)
	w.WriteInt32(1)
	return w.Bytes()
}

// TestShortFrameHeaderClosesWithServerClose sends an authenticated session a
// frame header shorter than the header itself: the server cannot read on,
// and the client reads ServerClose before the close. It runs on the wall
// clock, whose reads wait for frames the harness does not count.
func TestShortFrameHeaderClosesWithServerClose(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1),
		gameservertest.WithRealPool())

	if _, err := srv.Client.Conn().Write([]byte{0x01, 0x00}); err != nil {
		t.Fatalf("write short header: %v", err)
	}
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "short header ServerClose")
	srv.Client.ExpectClosed()
}
