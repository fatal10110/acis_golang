package lifecycle

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPanicInSelectionAfterRestoreDetachesPlayer panics the RequestGameStart
// handler as it answers CharSelected, after the selection restored the
// character, marked its row online and gave it a queue: the client reads
// ServerClose, then the close; the teardown detaches the restored player
// once, so its row is offline and its queue closed by the time the logout
// is reported, and a new selection of the character enters the world
// (#5278).
func TestPanicInSelectionAfterRestoreDetachesPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	openQueues := srv.OpenActorQueues()
	panicOnSend(t, srv, serverpackets.OpcodeCharSelected)

	srv.Client.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "after the panicked selection")
	srv.Client.ExpectClosed()
	srv.AwaitLoginLogout(t, srv.Account())

	if _, inWorld := srv.State.Player(objID); inWorld {
		t.Fatal("the panicked selection left the character in the world")
	}
	if got := srv.OpenActorQueues(); got != openQueues {
		t.Fatalf("open actor queues = %d, want %d: the restored player's queue was left open", got, openQueues)
	}
	var online int
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT online FROM characters WHERE obj_Id = ?", objID).Scan(&online); err != nil {
		t.Fatalf("read online: %v", err)
	}
	if online != 0 {
		t.Fatalf("online = %d after the panicked selection, want 0", online)
	}

	relog := srv.DialClient(t, srv.Account(), 1)
	startInWorld(t, relog)
	if _, inWorld := srv.State.Player(objID); !inWorld {
		t.Fatal("the character did not enter the world on its next selection")
	}
}
