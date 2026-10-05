package fishchamp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// heldWriter queues every job and runs none: a persistence lane backed up
// for as long as the test lasts.
type heldWriter struct{ jobs []func() }

func (w *heldWriter) Enqueue(_ int32, job func()) bool {
	w.jobs = append(w.jobs, job)
	return true
}

// failingStore fails every save while fail is set, and keeps the others.
type failingStore struct {
	recordingStore
	fail bool
}

func (s *failingStore) Save(ctx context.Context, end int64, entries []Entry) error {
	if s.fail {
		return errors.New("database down")
	}
	return s.recordingStore.Save(ctx, end, entries)
}

// claimTestChampionship restores a championship whose last week's winner
// is Angler, unclaimed, with its saves queued on writes.
func claimTestChampionship(t *testing.T, store Store, writes Writer) *Championship {
	t.Helper()
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cfg := Config{Enabled: true, RewardItemID: 57, Rewards: [Places]int32{100, 50, 30, 20, 10}}
	c := New(cfg, store, writes, sim.NewInline(start).NewQueue("fishchamp"), zerolog.Nop())
	if err := c.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return c
}

// The claim is in the database by the time Claim hands back the prize to
// pay, even with the championship's own lane backed up: the prize item's
// row, written whenever the item persistence gets to it, cannot land
// ahead of it, so a crash can lose the prize but never leave it claimable
// a second time (#3355).
func TestClaimIsStoredBeforeThePrizeIsPaid(t *testing.T) {
	store := &recordingStore{
		end:  time.Date(2026, 10, 6, 19, 0, 0, 0, time.UTC).UnixMilli(),
		rows: []Entry{{Name: "Angler", Length: 80, Reward: RewardUnclaimed}},
	}
	lane := &heldWriter{}
	c := claimTestChampionship(t, store, lane)

	paid, err := c.Claim(context.Background(), "angler")
	if err != nil || len(paid) != 1 || paid[0] != 100 {
		t.Fatalf("Claim = %v, %v; want [100]", paid, err)
	}
	n, rows := store.last()
	if n != 1 || len(rows) != 1 || rows[0].Reward != RewardClaimed {
		t.Fatalf("stored when the prize is paid: %d saves, last %+v; want Angler claimed", n, rows)
	}
	if len(lane.jobs) != 0 {
		t.Fatalf("the claim queued %d saves on the lane, want it stored at once", len(lane.jobs))
	}
}

// A claim that cannot be stored pays nothing and stays open: the winner
// claims it again once the database is back, and is paid once.
func TestClaimNotStoredPaysNothing(t *testing.T) {
	store := &failingStore{recordingStore: recordingStore{
		end:  time.Date(2026, 10, 6, 19, 0, 0, 0, time.UTC).UnixMilli(),
		rows: []Entry{{Name: "Angler", Length: 80, Reward: RewardUnclaimed}},
	}, fail: true}
	c := claimTestChampionship(t, store, &heldWriter{})

	if paid, err := c.Claim(context.Background(), "Angler"); err == nil || paid != nil {
		t.Fatalf("Claim with the database down = %v, %v; want nothing paid and the error", paid, err)
	}
	store.fail = false
	if paid, err := c.Claim(context.Background(), "Angler"); err != nil || len(paid) != 1 || paid[0] != 100 {
		t.Fatalf("Claim once the database is back = %v, %v; want [100]", paid, err)
	}
	if paid, err := c.Claim(context.Background(), "Angler"); err != nil || paid != nil {
		t.Fatalf("second Claim = %v, %v; want nothing", paid, err)
	}
	if n, rows := store.last(); n != 1 || rows[0].Reward != RewardClaimed {
		t.Fatalf("stored: %d saves, last %+v; want one save with Angler claimed", n, rows)
	}
}
