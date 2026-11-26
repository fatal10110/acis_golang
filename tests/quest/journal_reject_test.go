package quest

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestRejectedJournalWriteLosesOnlyItself sets a variable whose value is
// wider than its column between two writes that fit: the database refuses
// that one write, which alone is dropped, and the writes around it land in
// order. Nothing stays owed, so the next selection loads the journal.
func TestRejectedJournalWriteLosesOnlyItself(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, gameservertest.WithCapturedLog())
	enterWorld(t, srv)
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.NewState(c, sc)
		st.SetStatus(questlog.StatusStarted)
		st.Set("wide", strings.Repeat("x", 256))
		st.Set("ex", "2")
	})
	restart(t, srv)
	srv.FlushPersistence(t)

	want := map[string]string{"<state>": "STARTED", "ex": "2"}
	if got := questRows(t, srv, objID, q001); !maps.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if !strings.Contains(srv.LogText(), "quest journal write rejected") {
		t.Fatalf("log has no rejected write:\n%s", srv.LogText())
	}

	srv.Client.Send(clientwire.RequestGameStart(0))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelected)
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("selection after the rejected write did not register the player")
	}
}

// TestRejectedJournalBatchRetriesAfterOutage holds the journal's lane while a
// script sets the state, a, a value wider than its column and b, so one
// drain owes all four. The database refuses the batch; the one-write pass
// stores the state and a and drops the wide write, and then the database
// goes away at b: b stays owed, ahead of c set later. Once the database is
// back, the next drain stores b then c, nothing twice, and the character's
// selection is accepted.
func TestRejectedJournalBatchRetriesAfterOutage(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, gameservertest.WithCapturedLog())
	enterWorld(t, srv)

	release := srv.HoldPersistenceLane(t, objID)
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.NewState(c, sc)
		st.SetStatus(questlog.StatusStarted)
		st.Set("a", "1")
		st.Set("wide", strings.Repeat("x", 256))
		st.Set("b", "2")
	})
	// The refused batch, then the state, a and the wide write one at a time,
	// reach the database; b's own call is the first the outage fails.
	srv.FailJournalWritesAfter(4, errors.New("database away"))
	release()
	srv.FlushPersistence(t)

	wantFirst := []string{"upsert " + q001 + " | <state> | STARTED", "upsert " + q001 + " | a | 1"}
	if got := statementLines(srv.TakeJournalWrites()); !slices.Equal(got, wantFirst) {
		t.Fatalf("writes stored before the outage = %q, want %q", got, wantFirst)
	}
	if got, want := questRows(t, srv, objID, q001), map[string]string{"<state>": "STARTED", "a": "1"}; !maps.Equal(got, want) {
		t.Fatalf("rows during the outage = %v, want %v", got, want)
	}
	if log := srv.LogText(); !strings.Contains(log, "quest journal write rejected") || !strings.Contains(log, "database away") {
		t.Fatalf("log has no rejected write and failed drain:\n%s", log)
	}

	srv.FailJournalWrites(nil)
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		q.State(c, sc).Set("c", "3")
	})
	srv.FlushPersistence(t)

	wantLater := []string{"upsert " + q001 + " | b | 2", "upsert " + q001 + " | c | 3"}
	if got := statementLines(srv.TakeJournalWrites()); !slices.Equal(got, wantLater) {
		t.Fatalf("writes stored after the outage = %q, want %q", got, wantLater)
	}
	want := map[string]string{"<state>": "STARTED", "a": "1", "b": "2", "c": "3"}
	if got := questRows(t, srv, objID, q001); !maps.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	restart(t, srv)
	srv.Client.Send(clientwire.RequestGameStart(0))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelected)
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("selection after the outage did not register the player")
	}
}
