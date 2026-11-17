package quest

import (
	"maps"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
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

	srv.Client.Send(encodeRequestGameStart(0))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelected)
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("selection after the rejected write did not register the player")
	}
}
