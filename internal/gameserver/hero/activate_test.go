package hero

import (
	"context"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// upsertStore keeps the heroes rows SaveHeroes upserts.
type upsertStore struct {
	Store
	mu   sync.Mutex
	rows map[int32]Hero
}

func (s *upsertStore) SaveHeroes(_ context.Context, heroes map[int32]Hero) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	maps.Copy(s.rows, heroes)
	return nil
}

func (s *upsertStore) AddDiaryEntry(context.Context, int32, int64, int, int) error { return nil }

func (s *upsertStore) LoadDiary(context.Context, int32) ([]DiaryRow, error) { return nil, nil }

func (s *upsertStore) LoadFights(context.Context, int32, int64) ([]FightRow, error) { return nil, nil }

// heldWriter keeps every queued job until the test runs them.
type heldWriter struct{ jobs []func() }

func (w *heldWriter) Enqueue(_ int32, job func()) bool {
	w.jobs = append(w.jobs, job)
	return true
}

// TestActivateSavesTheHeroesAsTheyAreWhenItRuns pins that each claim's
// save stores the heroes when it runs: two claims whose saves reach the
// lane in the reverse order still leave both heroes active.
func TestActivateSavesTheHeroesAsTheyAreWhenItRuns(t *testing.T) {
	t.Parallel()
	store := &upsertStore{rows: map[int32]Hero{}}
	writes := &heldWriter{}
	m := New(store, nil, nil, writes, 5, time.Now, zerolog.Nop())
	m.heroes = map[int32]Hero{1: {ClassID: 88, Count: 1, Played: true}, 2: {ClassID: 89, Count: 1, Played: true}}

	for _, id := range []int32{1, 2} {
		if _, ok := m.Activate(id); !ok {
			t.Fatalf("Activate(%d) refused", id)
		}
	}
	for _, job := range slices.Backward(writes.jobs) {
		job()
	}
	for _, id := range []int32{1, 2} {
		if !store.rows[id].Active {
			t.Errorf("stored hero %d active = false, want true", id)
		}
	}
}
