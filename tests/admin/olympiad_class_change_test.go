package admin

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Occupation change and Olympiad waiting list system messages.
const (
	smClassTransfer          = 1308
	smDeletedFromWaitingList = 1505
)

// TestClassChangeDropsOlympiadRegistration pins the owner-approved fix of
// #5116, a divergence from keeping the registration under the class held
// when registering: once //set class moves a registered noble's base
// class, the noble is off the waiting list, told so after the class
// transfer, and its record and stored row count for the new class; it
// may then register under the new class, and only once whatever class it
// names.
func TestClassChangeDropsOlympiadRegistration(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(shippedAdminPages(t, "charclasses.htm")),
		gameservertest.WithOlympiadCompetition(time.Hour))
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	// The registration itself is driven through the manager in tests/npcs.
	applicant := olympiad.Applicant{ObjectID: userID, Name: "Player", Noble: true}
	if got := srv.Olympiad.Register(applicant, olympiad.Classed); got != olympiad.Registered {
		t.Fatalf("Register() = %v, want Registered", got)
	}

	exchange(t, gm, encodeBuildCmd("set class 1"))
	settle(t, user)
	srv.Advance(t, respawnDelay)
	var ids []int
	for _, f := range settle(t, user) {
		if id := systemMessageID(f); id >= 0 {
			ids = append(ids, id)
		}
	}
	if i := slices.Index(ids, smClassTransfer); i < 0 || !slices.Equal(ids[i:], []int{smClassTransfer, smDeletedFromWaitingList}) {
		t.Fatalf("system messages after the class change = %v, want CLASS_TRANSFER then YOU_HAVE_BEEN_DELETED_FROM_THE_WAITING_LIST_OF_A_GAME", ids)
	}
	settle(t, gm)
	if srv.Olympiad.IsRegistered(userID) || srv.Olympiad.IsRegisteredInComp(userID) {
		t.Fatal("still on the Olympiad waiting list once the base class moved")
	}
	if n, ok := srv.Olympiad.Noble(userID); !ok || n.ClassID != 1 {
		t.Fatalf("record = %+v, %v; want it kept under class 1", n, ok)
	}
	srv.FlushPersistence(t)
	var stored int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT class_id FROM olympiad_nobles WHERE char_id = ?", userID).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("stored class_id = %d, %v; want 1", stored, err)
	}

	applicant.BaseClass = 1
	if got := srv.Olympiad.Register(applicant, olympiad.Classed); got != olympiad.Registered {
		t.Fatalf("Register() under the new class = %v, want Registered", got)
	}
	applicant.BaseClass = 0
	for _, kind := range []olympiad.GameType{olympiad.Classed, olympiad.NonClassed} {
		if got := srv.Olympiad.Register(applicant, kind); got != olympiad.RegisterAlreadyClassed {
			t.Fatalf("Register(%v) under the old class = %v, want RegisterAlreadyClassed", kind, got)
		}
	}
	if got := srv.Olympiad.Unregister(userID, true); got != olympiad.Unregistered || srv.Olympiad.IsRegistered(userID) {
		t.Fatalf("Unregister() = %v, want Unregistered and off every list", got)
	}
}
