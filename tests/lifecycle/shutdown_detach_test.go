package lifecycle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestShutdownWaitsForConnectedPlayersDetach shuts the server down with a
// player still in game. Shutdown returns only once every connection has been
// detached and its saves have landed: a restart test boots a second stack on
// the same database right after, and a detach save of the first stack still
// running then would overwrite what the second stack writes with the first
// stack's state (#3841). The characters row is therefore already marked
// offline when Shutdown returns.
func TestShutdownWaitsForConnectedPlayersDetach(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)

	srv.Shutdown(t)

	if _, online := persistedHPAndOnline(t, srv, objID); online != 0 {
		t.Fatalf("characters row online = %d right after Shutdown, want 0 (detach save still pending)", online)
	}
}
