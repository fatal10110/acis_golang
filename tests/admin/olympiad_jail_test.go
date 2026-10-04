package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestJailDropsOlympiadRegistration pins Punishment.setType JAIL's
// removeDisconnectedCompetitor: a player waiting for an Olympiad match is
// taken off the waiting list when jailed.
func TestJailDropsOlympiadRegistration(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(punishPages(t)),
		gameservertest.WithZones(shippedJailZones(t)),
		gameservertest.WithOlympiadCompetition(time.Hour))
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	// The registration itself is driven through the manager in tests/npcs.
	if got := srv.Olympiad.Register(olympiad.Applicant{ObjectID: userID, Name: "Player", Noble: true}, olympiad.NonClassed); got != olympiad.Registered {
		t.Fatalf("Register() = %v, want Registered", got)
	}
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("jail Player 30"))), "Player is jailed for 30 minutes.")
	settle(t, user)
	waitPunishment(t, srv, userID, punishJail)
	if srv.Olympiad.IsRegistered(userID, 0) {
		t.Fatal("still on the Olympiad waiting list once jailed")
	}
}
