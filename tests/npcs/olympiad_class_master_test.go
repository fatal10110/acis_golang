package npcs

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestClassManagerChangeDropsOlympiadRegistration pins the owner-approved
// fix of #5116 at the class manager: a noble waiting for a classed game
// who changes occupation there is told of the class transfer, then that
// it left the waiting list; it is off every list, and its record and
// stored row count for the new class.
func TestClassManagerChangeDropsOlympiadRegistration(t *testing.T) {
	w := bootClassManagerWorld(t, classPrice, gameservertest.WithOlympiadCompetition(time.Hour))
	// The registration itself is driven through the Olympiad manager in
	// olympiad_manager_test.go; only the class change is under test here.
	applicant := olympiad.Applicant{ObjectID: w.player, Name: "Talker", Noble: true}
	if got := w.srv.Olympiad.Register(applicant, olympiad.Classed); got != olympiad.Registered {
		t.Fatalf("Register() = %v, want Registered", got)
	}
	w.command(t, "1stClass")

	frames := w.command(t, "change_class "+strconv.Itoa(warriorOccupation))
	var ids []int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, systemMessageID(f))
		}
	}
	if i := slices.Index(ids, smClassTransfer); i < 0 || i+1 >= len(ids) || ids[i+1] != smDeletedFromWaitingList {
		t.Fatalf("system messages = %v, want CLASS_TRANSFER then YOU_HAVE_BEEN_DELETED_FROM_THE_WAITING_LIST_OF_A_GAME", ids)
	}
	if w.srv.Olympiad.IsRegistered(w.player) || w.srv.Olympiad.IsRegisteredInComp(w.player) {
		t.Fatal("still on the Olympiad waiting list once the occupation changed")
	}
	if n, ok := w.srv.Olympiad.Noble(w.player); !ok || n.ClassID != warriorOccupation {
		t.Fatalf("record = %+v, %v; want it under class %d", n, ok, warriorOccupation)
	}
	w.srv.FlushPersistence(t)
	var stored int
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT class_id FROM olympiad_nobles WHERE char_id = ?", w.player).Scan(&stored); err != nil || stored != warriorOccupation {
		t.Fatalf("stored class_id = %d, %v; want %d", stored, err, warriorOccupation)
	}

	// Registered again under the new class, the noble holds one place
	// only, whatever class it names.
	applicant.BaseClass = warriorOccupation
	if got := w.srv.Olympiad.Register(applicant, olympiad.Classed); got != olympiad.Registered {
		t.Fatalf("Register() under the new class = %v, want Registered", got)
	}
	applicant.BaseClass = 0
	if got := w.srv.Olympiad.Register(applicant, olympiad.NonClassed); got != olympiad.RegisterAlreadyClassed {
		t.Fatalf("Register(NonClassed) under the old class = %v, want RegisterAlreadyClassed", got)
	}
}
