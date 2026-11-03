package hero

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// electionStore picks one noble per class, the class's object id in
// picks, for the election tests.
type electionStore struct {
	recordStore
	picks map[int]int32
}

func (s *electionStore) BestNoble(_ context.Context, classID, _ int) (int32, string, bool, error) {
	id, ok := s.picks[classID]
	return id, fmt.Sprint("h", id), ok, nil
}

func (s *electionStore) DeleteHeroItems(context.Context) error { return nil }

func (s *electionStore) ClanID(context.Context, int32) (int32, bool, error) { return 0, false, nil }

func newElectionManager(store *electionStore) *Manager {
	return New(store, nil, fixedNames{}, nil, 5, func() time.Time { return recordsNow }, zerolog.Nop())
}

// electedOrder runs an election picking ids for the classes from 88 on,
// in that order, and returns the hero list's object ids.
func electedOrder(t *testing.T, m *Manager, store *electionStore, ids ...int32) []int32 {
	t.Helper()
	store.picks = make(map[int]int32, len(ids))
	byName := make(map[string]int32, len(ids))
	for k, id := range ids {
		store.picks[88+k] = id
		byName[fmt.Sprint("h", id)] = id
	}
	m.Elect(context.Background())
	var out []int32
	for _, h := range m.Heroes() {
		id, ok := byName[h.Name]
		if !ok {
			t.Fatalf("hero %q was not elected", h.Name)
		}
		out = append(out, id)
	}
	return out
}

// TestElectionListsHeroesInTableOrder pins the hero list after an
// election: the picks, gathered in class order, are copied into the hero
// table, which a first election sizes for the copy (4 buckets for 3 heroes)
// and a later one keeps at the size it grew to.
func TestElectionListsHeroesInTableOrder(t *testing.T) {
	t.Parallel()
	t.Run("first election", func(t *testing.T) {
		t.Parallel()
		store := &electionStore{}
		m := newElectionManager(store)
		// Gathered in 16 buckets as [1 2 5]; the 4-bucket copy lists 5 in
		// 1's bucket.
		if got, want := electedOrder(t, m, store, 1, 5, 2), []int32{1, 5, 2}; !slices.Equal(got, want) {
			t.Fatalf("heroes = %v, want %v", got, want)
		}
	})
	t.Run("later election keeps the grown size", func(t *testing.T) {
		t.Parallel()
		store := &electionStore{}
		m := newElectionManager(store)
		first := make([]int32, 13)
		for k := range first {
			first[k] = 0x40 + int32(k)
		}
		if got := electedOrder(t, m, store, first...); !slices.Equal(got, first) {
			t.Fatalf("first heroes = %x, want %x", got, first)
		}
		// Gathered in 16 buckets as [0x11 0x01] (one bucket); the kept
		// 32-bucket table splits them, where a fresh 4-bucket one would not.
		if got, want := electedOrder(t, m, store, 0x11, 0x01), []int32{0x01, 0x11}; !slices.Equal(got, want) {
			t.Fatalf("second heroes = %x, want %x", got, want)
		}
	})
}
