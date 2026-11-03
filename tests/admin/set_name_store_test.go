package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// rowName reads objID's char_name as it is now, without waiting for the
// persistence lanes.
func rowName(t *testing.T, srv *gameservertest.Server, objID int32) string {
	t.Helper()
	var name string
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT char_name FROM characters WHERE obj_Id = ?", objID).Scan(&name); err != nil {
		t.Fatalf("read char_name of %d: %v", objID, err)
	}
	return name
}

// TestAdminSetPlayerNameStoredBeforeReported pins that a rename has taken
// the name in the characters table, which character creation checks, by
// the time the GM is told, even while the player's persistence lane is
// busy: the name is reserved at once, as the reference's name table is.
func TestAdminSetPlayerNameStoredBeforeReported(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	release := srv.HoldPersistenceLane(t, userID)
	frames := append(exchange(t, gm, encodeBuildCmd("set name Renamed7")), settle(t, gm)...)
	_, rest := split(frames)
	assertTexts(t, rest, "You successfully set your target's name to Renamed7.")
	if got := rowName(t, srv, userID); got != "Renamed7" {
		t.Fatalf("char_name when the GM is told = %q, want Renamed7", got)
	}
	release()
	if got := storedName(t, srv, userID); got != "Renamed7" {
		t.Fatalf("stored char_name = %q, want Renamed7", got)
	}
}

// TestAdminSetPlayerNameLookupFailure pins that a name the characters
// table cannot be asked about is refused with INCORRECT_NAME_TRY_AGAIN and
// changes nothing.
func TestAdminSetPlayerNameLookupFailure(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithCharacterNameFault(gameservertest.CharacterNameFault{Lookup: errors.New("characters table down")}))
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	frames := append(exchange(t, gm, encodeBuildCmd("set name Renamed7")), settle(t, gm)...)
	if !incorrectName(frames) {
		t.Fatalf("GM frames = %x, want INCORRECT_NAME_TRY_AGAIN alone", testsupport.FrameOpcodes(frames))
	}
	assertNotRenamed(t, srv, user, userID)
}

// TestAdminSetPlayerNameStoreFailure pins that a name the characters table
// does not take is refused with INCORRECT_NAME_TRY_AGAIN: the player keeps
// its old name, is still found by it, and nobody is shown the new one.
func TestAdminSetPlayerNameStoreFailure(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithCharacterNameFault(gameservertest.CharacterNameFault{Store: errors.New("characters table down")}))
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	frames := append(exchange(t, gm, encodeBuildCmd("set name Renamed7")), settle(t, gm)...)
	if !incorrectName(frames) {
		t.Fatalf("GM frames = %x, want INCORRECT_NAME_TRY_AGAIN alone", testsupport.FrameOpcodes(frames))
	}
	assertNotRenamed(t, srv, user, userID)
}

// assertNotRenamed asserts the player named Player was shown nothing and
// is still Player in the world, the name index and its row.
func assertNotRenamed(t *testing.T, srv *gameservertest.Server, user *testsupport.ScriptedClient, userID int32) {
	t.Helper()
	if frames := settle(t, user); len(frames) != 0 {
		t.Fatalf("player frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	if got := onlineCharacter(t, srv, userID).CharacterName(); got != "Player" {
		t.Fatalf("player name = %q, want Player", got)
	}
	if p, ok := srv.State.PlayerByName("Player"); !ok || p.ObjectID() != userID {
		t.Fatal("the player is not found by its name")
	}
	if _, ok := srv.State.PlayerByName("Renamed7"); ok {
		t.Fatal("the player is found by the refused name")
	}
	if got := storedName(t, srv, userID); got != "Player" {
		t.Fatalf("stored char_name = %q, want Player", got)
	}
}
