package lifecycle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// selectionWaitMsg is the message of the warning a selection logs once its
// wait for queued saves passes half its budget.
const selectionWaitMsg = "select character: wait for queued saves passed half its budget"

// TestFastSelectionWaitDoesNotWarn selects a character whose lane is idle,
// under the default wait budget: the selection answers and logs no slow-wait
// warning.
func TestFastSelectionWaitDoesNotWarn(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
	)
	startInWorld(t, srv.Client)
	if got := selectionWaitWarnings(t, srv); len(got) != 0 {
		t.Fatalf("fast selection logged %d slow-wait warnings: %v", len(got), got)
	}
}

// assertSelectionWaitWarned checks that exactly one slow-wait warning was
// logged for objID, naming its persistence lane and the owner.
func assertSelectionWaitWarned(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	got := selectionWaitWarnings(t, srv)
	if len(got) != 1 {
		t.Fatalf("slow-wait warnings = %d (%v), want 1", len(got), got)
	}
	w := got[0]
	if w.Level != "warn" || w.Lane == nil || *w.Lane != persist.LaneIndex(objID) || w.OwnerID != objID {
		t.Fatalf("slow-wait warning = %+v, want level warn, lane %d, owner_id %d", w, persist.LaneIndex(objID), objID)
	}
}

type selectionWaitWarning struct {
	Level   string  `json:"level"`
	Lane    *uint32 `json:"lane"`
	OwnerID int32   `json:"owner_id"`
	Budget  float64 `json:"budget"`
}

func selectionWaitWarnings(t *testing.T, srv *gameservertest.Server) []selectionWaitWarning {
	t.Helper()
	var out []selectionWaitWarning
	for line := range strings.SplitSeq(srv.LogText(), "\n") {
		if !strings.Contains(line, selectionWaitMsg) {
			continue
		}
		var w selectionWaitWarning
		if err := json.Unmarshal([]byte(line), &w); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		out = append(out, w)
	}
	return out
}
