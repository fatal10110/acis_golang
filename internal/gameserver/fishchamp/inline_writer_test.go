package fishchamp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// recordingStore keeps every save it is handed.
type recordingStore struct {
	mu    sync.Mutex
	end   int64
	rows  []Entry
	saves [][]Entry
}

func (s *recordingStore) Load(context.Context) (int64, []Entry, error) {
	return s.end, s.rows, nil
}

func (s *recordingStore) Save(_ context.Context, _ int64, entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves = append(s.saves, entries)
	return nil
}

func (s *recordingStore) last() (int, []Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.saves) == 0 {
		return 0, nil
	}
	return len(s.saves), s.saves[len(s.saves)-1]
}

// A writer that runs its jobs inline, as a nil *persist.Worker does, must
// not deadlock the change that queued the save: the save takes the lock
// the change holds.
func TestChangesSaveThroughAnInlineWriter(t *testing.T) {
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	store := &recordingStore{
		end:  start.Add(-time.Hour).UnixMilli(),
		rows: []Entry{{Name: "Angler", Length: 70, Reward: RewardNone}},
	}
	var inline *persist.Worker
	cfg := Config{Enabled: true, RewardItemID: 57, Rewards: [Places]int32{100, 50, 30, 20, 10}}
	c := New(cfg, store, inline, sim.NewInline(start).NewQueue("fishchamp"), zerolog.Nop(),
		WithRoll(func(int) int { return 0 }))

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := c.Restore(context.Background()); err != nil {
			t.Errorf("Restore: %v", err)
			return
		}
		c.Start() // the week is over: it ends, and its fisher wins
		if n, rows := store.last(); n != 1 || len(rows) != 1 || rows[0].Reward != RewardUnclaimed {
			t.Errorf("after the week end: %d saves, last %+v; want 1 save with Angler unclaimed", n, rows)
		}
		if paid := c.Claim("angler"); len(paid) != 1 || paid[0] != 100 {
			t.Errorf("Claim = %v, want [100]", paid)
		}
		if n, rows := store.last(); n != 2 || len(rows) != 1 || rows[0].Reward != RewardClaimed {
			t.Errorf("after the claim: %d saves, last %+v; want 2 saves with Angler claimed", n, rows)
		}
		if catch, ok := c.NewFish("Rod", 0); !ok || !catch.Registered {
			t.Errorf("NewFish = %+v, %v; want a registered catch", catch, ok)
		}
		if n, rows := store.last(); n != 3 || len(rows) != 2 || rows[1].Name != "Rod" || rows[1].Reward != RewardNone {
			t.Errorf("after the catch: %d saves, last %+v; want 3 saves with Rod running", n, rows)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a change deadlocked saving through an inline writer")
	}
}
