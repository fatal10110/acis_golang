package lifecycle

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestConcurrentSelectionOfOneCharacterIsRefused pins one selection of a
// character at a time. A first connection's selection waits on the
// character's held persistence lane; a second connection of the account takes
// the account over and selects the same character while the first is still
// under way. The second selection is refused silently, so once the lane is
// released only the first one registers the character, and its departure (its
// connection was taken over) leaves nothing of the character behind that
// another session could lose. The second connection then selects and enters
// the world, and stays registered: found by id and by name, and autosaved.
func TestConcurrentSelectionOfOneCharacterIsRefused(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Twin", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	objID := srv.SoleObjectID(t)
	first := srv.Client

	release := srv.HoldPersistenceLane(t, objID)
	first.Send(clientwire.RequestGameStart(0))
	if got := srv.ReadQueued(t, first); len(got) != 0 {
		t.Fatalf("first selection answered %d frames while its saves were held, want it waiting", len(got))
	}

	second := srv.DialClient(t, srv.Account(), 1)
	second.Send(clientwire.RequestGameStart(0))
	if got := srv.ReadQueued(t, second); len(got) != 0 {
		t.Fatalf("second selection answered %d frames while the first was under way, want a silent refusal", len(got))
	}
	release()

	// The first selection registers the character and its taken-over
	// connection then takes it out of the world again; until it has, the
	// second connection's selection is refused as a duplicate login.
	srv.AdvanceUntil(t, "second connection's selection", func() bool {
		second.Send(clientwire.RequestGameStart(0))
		frames := srv.ReadQueued(t, second)
		return slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeCharSelected })
	})
	second.Send(clientwire.EnterWorld())
	readEnterWorldBurst(t, second)
	drainUntilQuiet(t, second)

	if _, online := srv.State.Player(objID); !online {
		t.Fatal("character not registered after the second connection entered the world")
	}
	if _, ok := srv.State.PlayerByName("Twin"); !ok {
		t.Fatal("character not found by name after the second connection entered the world")
	}
	srv.DamagePlayerHP(t, objID, 5)
	autosaveCycle(t, srv)
	if got, want := outageCurHP(t, srv.DB, objID), srv.PlayerCurrentHP(t, objID); got != want {
		t.Fatalf("stored HP = %d after an autosave cycle, want %d: the entered session is not autosaved", got, want)
	}
}
