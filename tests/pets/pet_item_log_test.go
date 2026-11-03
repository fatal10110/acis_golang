package pets

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// petItemRecord is one item log line.
type petItemRecord struct {
	State    string `json:"message"`
	Owner    string `json:"owner"`
	OwnerID  int32  `json:"owner_id"`
	ObjectID int32  `json:"object_id"`
	Location string `json:"location"`
}

// petItemLog is the item log the server writes while the test reads it.
type petItemLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *petItemLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// take returns every record written since the last take.
func (l *petItemLog) take(t *testing.T) []petItemRecord {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []petItemRecord
	for _, line := range strings.Split(strings.TrimSpace(l.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var r petItemRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("item log record %q: %v", line, err)
		}
		out = append(out, r)
	}
	l.buf.Reset()
	return out
}

// TestItemLogRecordsPetInventoryChanges pins LogItems on a pet's inventory:
// the wolf putting on its weapon is a change recorded under the collar, and
// taking the worn weapon back records the take-off in the pet's inventory,
// then the weapon leaving it, before it arrives in the owner's inventory.
func TestItemLogRecordsPetInventoryChanges(t *testing.T) {
	t.Parallel()
	var log petItemLog
	h, _, weaponID := bootArmedWolf(t, gameservertest.WithItemLog(zerolog.New(&log)))
	log.take(t)

	h.sendAndTick(t, encodeRequestPetUseItem(weaponID))
	want := []petItemRecord{{"MODIFIED", "Owner", h.collarID, weaponID, "PET_EQUIP"}}
	if got := log.take(t); !slices.Equal(got, want) {
		t.Fatalf("item log after the wolf equips = %+v, want %+v", got, want)
	}

	h.sendAndTick(t, encodeRequestGetItemFromPet(weaponID, 1))
	want = []petItemRecord{
		{"MODIFIED", "Owner", h.collarID, weaponID, "PET"},
		{"REMOVED", "Owner", h.collarID, weaponID, "PET"},
		{"ADDED", "Owner", h.ownerID, weaponID, "INVENTORY"},
	}
	if got := log.take(t); !slices.Equal(got, want) {
		t.Fatalf("item log after the take-back = %+v, want %+v", got, want)
	}
}
