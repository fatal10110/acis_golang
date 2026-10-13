package gameservertest

import (
	"io"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// dropNoticeTimeout bounds the wall time the server may take to notice a
// closed connection.
const dropNoticeTimeout = 5 * time.Second

// DropInCombat puts objID in combat and closes its connection conn, so the
// player lingers in the world with its client detached instead of leaving
// at once. It returns once the server has noticed the drop, the player
// still in the world.
//
// The drop is noticed on the connection's own goroutine, not on a timer, so
// DropInCombat waits for it in wall time and lets no time pass on a driven
// clock: the linger runs from the drop itself.
func (s *Server) DropInCombat(tb testing.TB, objID int32, conn io.Closer) {
	tb.Helper()
	s.SetPlayerInCombat(tb, objID, true)
	obj, ok := s.State.Player(objID)
	if !ok {
		tb.Fatalf("player %d missing from the world before the drop", objID)
	}
	if err := conn.Close(); err != nil {
		tb.Fatalf("close player %d's connection: %v", objID, err)
	}
	for deadline := time.Now().Add(dropNoticeTimeout); !network.ClientDetached(obj); {
		if time.Now().After(deadline) {
			tb.Fatalf("the server never noticed player %d's connection drop", objID)
		}
		time.Sleep(time.Millisecond)
	}
	if _, ok := s.State.Player(objID); !ok {
		tb.Fatalf("player %d, dropped in combat, left the world at once", objID)
	}
}
