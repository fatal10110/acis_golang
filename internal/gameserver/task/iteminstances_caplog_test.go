package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// capLogEntry is the subset of a logged JSON line the cap tests read.
type capLogEntry struct {
	Level     string  `json:"level"`
	Message   string  `json:"message"`
	Dropped   int     `json:"dropped"`
	ObjectIDs []int32 `json:"object_ids"`
	Pending   int     `json:"pending"`
	Cap       int     `json:"cap"`
	Error     string  `json:"error"`
}

func capLogEntries(t *testing.T, buf *bytes.Buffer, level, msgPrefix string) []capLogEntry {
	t.Helper()
	var out []capLogEntry
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry capLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if entry.Level == level && strings.HasPrefix(entry.Message, msgPrefix) {
			out = append(out, entry)
		}
	}
	return out
}

const (
	capDropMsg  = "task: item persistence backlog at its cap"
	capWarnMsg  = "task: item persistence backlog is past half its cap"
	capFlushErr = "database unavailable"
)

// addOwnedPending adds n items, each under its own owner, so a Save runs one
// owner job per item.
func addOwnedPending(instances *ItemInstances, n int) {
	for id := int32(1); id <= int32(n); id++ {
		instances.Add(ownedItem(id, id))
	}
}

// TestItemInstancesPendingCapDropIsLoggedOncePerRound pins #2301's
// operational signal: a Save whose failed writes overflow the cap logs one
// Error naming the dropped count, their ids, the pending count and the cap,
// even when the drops come from several owner jobs.
func TestItemInstancesPendingCapDropIsLoggedOncePerRound(t *testing.T) {
	var buf bytes.Buffer
	flusher := &outcomeFlusher{onFlush: func(context.Context, int) error { return errors.New(capFlushErr) }}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.New(&buf))
	instances.pendingCap = 3
	addOwnedPending(instances, 5)

	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the flush error")
	}

	drops := capLogEntries(t, &buf, "error", capDropMsg)
	if len(drops) != 1 {
		t.Fatalf("cap-drop Error lines = %d, want exactly one for the round:\n%s", len(drops), buf.String())
	}
	got := drops[0]
	if got.Dropped != 2 || !slices.Equal(got.ObjectIDs, []int32{4, 5}) || got.Pending != 3 || got.Cap != 3 {
		t.Fatalf("cap-drop line = %+v, want dropped=2 object_ids=[4 5] pending=3 cap=3", got)
	}
	if got.Error != capFlushErr {
		t.Fatalf("cap-drop line error = %q, want the flush error %q", got.Error, capFlushErr)
	}
}

// TestItemInstancesPendingCapDropSampleIsBounded pins that the drop line
// names at most capDropSample ids while still counting every drop.
func TestItemInstancesPendingCapDropSampleIsBounded(t *testing.T) {
	var buf bytes.Buffer
	flusher := &outcomeFlusher{onFlush: func(context.Context, int) error { return errors.New(capFlushErr) }}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.New(&buf))
	instances.pendingCap = 1
	addOwnedPending(instances, capDropSample+6)

	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the flush error")
	}
	drops := capLogEntries(t, &buf, "error", capDropMsg)
	if len(drops) != 1 {
		t.Fatalf("cap-drop Error lines = %d, want one:\n%s", len(drops), buf.String())
	}
	if got := drops[0]; got.Dropped != capDropSample+5 || !slices.Equal(got.ObjectIDs, idRange(2, capDropSample+1)) {
		t.Fatalf("cap-drop line = %+v, want dropped=%d and the first %d dropped ids", got, capDropSample+5, capDropSample)
	}
}

// TestItemInstancesNoCapDropNoErrorLine pins the other side: a failed round
// that stays under the cap keeps every row and logs no drop.
func TestItemInstancesNoCapDropNoErrorLine(t *testing.T) {
	var buf bytes.Buffer
	flusher := &outcomeFlusher{onFlush: func(context.Context, int) error { return errors.New(capFlushErr) }}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.New(&buf))
	instances.pendingCap = 5
	addOwnedPending(instances, 5)

	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the flush error")
	}
	if drops := capLogEntries(t, &buf, "error", capDropMsg); len(drops) != 0 {
		t.Fatalf("cap-drop Error lines = %+v, want none with the backlog at the cap but not past it", drops)
	}
}

// TestItemInstancesHalfCapBacklogWarns pins the early warning Save gives
// before anything is dropped: a Warn once the backlog it takes reaches half
// the cap, and silence below that.
func TestItemInstancesHalfCapBacklogWarns(t *testing.T) {
	for _, tc := range []struct {
		backlog  int
		wantWarn bool
	}{
		{backlog: 4, wantWarn: false},
		{backlog: 5, wantWarn: true},
		{backlog: 7, wantWarn: true},
	} {
		var buf bytes.Buffer
		instances := NewItemInstances(&outcomeFlusher{}, item.NewTable(nil), nil, nil, zerolog.New(&buf))
		instances.pendingCap = 10
		addOwnedPending(instances, tc.backlog)

		if err := instances.Save(context.Background()); err != nil {
			t.Fatalf("backlog %d: Save() = %v", tc.backlog, err)
		}
		warns := capLogEntries(t, &buf, "warn", capWarnMsg)
		if !tc.wantWarn {
			if len(warns) != 0 {
				t.Fatalf("backlog %d of cap 10: Warn lines = %+v, want none below half the cap", tc.backlog, warns)
			}
			continue
		}
		if len(warns) != 1 || warns[0].Pending != tc.backlog || warns[0].Cap != 10 {
			t.Fatalf("backlog %d of cap 10: Warn lines = %+v, want one with pending=%d cap=10", tc.backlog, warns, tc.backlog)
		}
	}
}
