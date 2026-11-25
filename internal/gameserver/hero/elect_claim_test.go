package hero

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// claimStore has hero 1 claim its status while the election reads, and
// fails every read when fail is set.
type claimStore struct {
	recordStore
	m       *Manager
	fail    bool
	claimed []bool
}

func (s *claimStore) BestNoble(context.Context, int, int) (int32, string, bool, error) {
	_, ok := s.m.Activate(1)
	s.claimed = append(s.claimed, ok)
	if s.fail {
		return 0, "", false, errors.New("injected read fault")
	}
	return 0, "", false, nil
}

// TestElectionHoldsClaims pins the claims around an election (#4697): an
// outgoing hero cannot claim its status while the election runs; when
// every attempt fails, the era keeps its heroes and the claim succeeds
// afterwards.
func TestElectionHoldsClaims(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		store := &claimStore{fail: fail, recordStore: recordStore{
			rows:    []Row{{ObjectID: 1, Name: "One", ClassID: 88, Count: 1, Played: true}},
			diaries: map[int32][]DiaryRow{},
		}}
		store.m = New(store, nil, fixedNames{}, nil, 5, func() time.Time { return recordsNow }, zerolog.Nop())
		if err := store.m.Restore(context.Background()); err != nil {
			t.Fatal(err)
		}
		store.m.Elect(1)
		for _, ok := range store.claimed {
			if ok {
				t.Fatalf("fail=%v: a claim succeeded while the election ran", fail)
			}
		}
		if got := store.m.IsInactive(1); got != fail {
			t.Fatalf("fail=%v: IsInactive(1) after the election = %v, want %v", fail, got, fail)
		}
		if _, ok := store.m.Activate(1); ok != fail {
			t.Fatalf("fail=%v: claim after the election = %v, want %v", fail, ok, fail)
		}
	}
}
