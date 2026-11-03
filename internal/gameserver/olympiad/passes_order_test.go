package olympiad

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// The tests below pin the order of a noblesse pass claim against the
// passes it pays (#3454): the claim's olympiad_nobles row lands before
// Passes returns anything to pay, so the passes' item row, written later
// on the player's lane, can never reach the database first. A crash before
// the row lands restarts the noble unclaimed and without passes; one after
// it restarts the noble claimed.

// heldWriter queues every job and runs none until run; refuse drops them.
type heldWriter struct {
	mu     sync.Mutex
	jobs   []func()
	queued chan struct{}
	refuse bool
}

func (w *heldWriter) Enqueue(_ int32, job func()) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.refuse {
		return false
	}
	w.jobs = append(w.jobs, job)
	if w.queued != nil {
		w.queued <- struct{}{}
	}
	return true
}

func (w *heldWriter) Flush(context.Context, ...int32) error { return nil }

func (w *heldWriter) run() {
	w.mu.Lock()
	jobs := w.jobs
	w.jobs = nil
	w.mu.Unlock()
	for _, job := range jobs {
		job()
	}
}

// failingStore is a recordingStore whose SaveNobles fails while fail is
// set.
type failingStore struct {
	recordingStore
	fail bool
}

func (s *failingStore) SaveNobles(ctx context.Context, nobles map[int32]Noble) error {
	if s.fail {
		return errors.New("database down")
	}
	return s.recordingStore.SaveNobles(ctx, nobles)
}

// passesOlympiad returns an Olympiad in validation holding noble 1 with
// 100 points, persisting to store through writes.
func passesOlympiad(store Store, writes Writer) *Olympiad {
	o := New(DefaultConfig(), store, writes, silentAnnouncer{}, nil, sim.NewInline(testNow).NewQueue("olympiad"), zerolog.Nop())
	o.period, o.periodEnd = Validation, testNow.Add(time.Hour).UnixMilli()
	o.nobles = map[int32]Noble{1: {ClassID: 88, Points: 100}}
	return o
}

func TestPassesPayOnlyOnceTheClaimLanded(t *testing.T) {
	t.Parallel()
	store := &recordingStore{}
	writes := &heldWriter{queued: make(chan struct{}, 1)}
	o := passesOlympiad(store, writes)

	type claim struct {
		passes int
		err    error
	}
	done := make(chan claim, 1)
	go func() {
		passes, err := o.Passes(context.Background(), 1, false)
		done <- claim{passes, err}
	}()
	<-writes.queued
	select {
	case c := <-done:
		t.Fatalf("Passes returned %d, %v before the claim's row landed", c.passes, c.err)
	default:
	}
	// A crash now: the database holds no claim and the player no passes.
	if len(store.saves) != 0 {
		t.Fatalf("saves before the lane ran = %+v", store.saves)
	}

	writes.run()
	c := <-done
	if c.err != nil || c.passes != 100000 {
		t.Fatalf("Passes = %d, %v; want 100000 once the row landed", c.passes, c.err)
	}
	want := Noble{ClassID: 88, Rewarded: true}
	if len(store.saves) != 1 || store.saves[0][1] != want {
		t.Fatalf("saves = %+v, want the claimed record", store.saves)
	}
}

func TestPassesClaimNotStoredPaysNothing(t *testing.T) {
	t.Parallel()
	unclaimed := Noble{ClassID: 88, Points: 100}
	for _, tc := range []struct {
		name   string
		store  *failingStore
		writes Writer
	}{
		{"write fails", &failingStore{fail: true}, nil},
		{"write dropped", &failingStore{}, &heldWriter{refuse: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := passesOlympiad(tc.store, tc.writes)
			if passes, err := o.Passes(context.Background(), 1, false); err == nil || passes != 0 {
				t.Fatalf("Passes = %d, %v; want 0 and the store's error", passes, err)
			}
			if n, _ := o.Noble(1); n != unclaimed {
				t.Fatalf("record after an unstored claim = %+v, want it unclaimed: %+v", n, unclaimed)
			}
		})
	}

	// A claim that could not wait for its row pays nothing either and is
	// open again; its row landing later costs the noble the claim, never
	// pays it twice.
	store := &recordingStore{}
	writes := &heldWriter{}
	o := passesOlympiad(store, writes)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if passes, err := o.Passes(ctx, 1, false); !errors.Is(err, context.Canceled) || passes != 0 {
		t.Fatalf("Passes past its deadline = %d, %v; want 0 and the deadline", passes, err)
	}
	if n, _ := o.Noble(1); n != unclaimed {
		t.Fatalf("record after a claim past its deadline = %+v, want it unclaimed: %+v", n, unclaimed)
	}
	writes.run()
	if len(store.saves) != 1 || !store.saves[0][1].Rewarded {
		t.Fatalf("saves = %+v, want the late claim's row", store.saves)
	}
}
