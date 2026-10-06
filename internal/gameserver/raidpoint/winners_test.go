package raidpoint

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/rs/zerolog"
)

// opStore records the writes it is asked for, in order.
type opStore struct{ ops []string }

func (s *opStore) Load(context.Context) ([]Row, error) { return nil, nil }
func (s *opStore) Save(_ context.Context, row Row) error {
	s.ops = append(s.ops, fmt.Sprintf("save %d %d %d", row.CharID, row.BossID, row.Points))
	return nil
}

func (s *opStore) Clear(context.Context) error {
	s.ops = append(s.ops, "clear")
	return nil
}

// laneWriter queues jobs per lane and runs them only when asked.
type laneWriter struct{ lanes map[int32][]func() }

func (w *laneWriter) Enqueue(ownerID int32, job func()) bool {
	w.lanes[ownerID] = append(w.lanes[ownerID], job)
	return true
}

// TestWinnersRanksFirstHundred: players with no points are left out, the
// highest totals come first, equal totals by object id, and no more than
// 100 are returned.
func TestWinnersRanksFirstHundred(t *testing.T) {
	p := New(&opStore{}, nil, zerolog.Nop())
	for id := int32(1); id <= 120; id++ {
		p.Add(id, 25001, id%50)
	}
	got := p.Winners()
	if len(got) != 100 {
		t.Fatalf("%d winners, want 100", len(got))
	}
	// Totals 49 (ids 49, 99), 48 (48, 98), ...
	if want := []int32{49, 99, 48, 98, 47, 97}; !slices.Equal(got[:6], want) {
		t.Fatalf("first winners %v, want %v", got[:6], want)
	}
	for i, id := range got {
		if r := p.Record(id); r.Rank != int32(i+1) {
			t.Fatalf("winner %d (player %d) has rank %d in its record", i+1, id, r.Rank)
		}
		if id%50 == 0 {
			t.Fatalf("player %d with no points is a winner", id)
		}
	}
}

// TestCleanUpQueuesBehindEarlierTotals: the wipe is queued on the same
// lane after every total stored before it, and the points are forgotten
// at once.
func TestCleanUpQueuesBehindEarlierTotals(t *testing.T) {
	store := &opStore{}
	writes := &laneWriter{lanes: map[int32][]func(){}}
	p := New(store, writes, zerolog.Nop())
	p.Add(7, 25001, 10)
	p.CleanUp()
	p.Add(7, 25001, 3)
	if r := p.Record(7); r.Total != 3 {
		t.Fatalf("total after the wipe and 3 more = %d, want 3", r.Total)
	}
	if len(writes.lanes) != 1 {
		t.Fatalf("writes on %d lanes, want one", len(writes.lanes))
	}
	for _, job := range writes.lanes[writeLane] {
		job()
	}
	if want := []string{"save 7 25001 10", "clear", "save 7 25001 3"}; !slices.Equal(store.ops, want) {
		t.Fatalf("stored %v, want %v", store.ops, want)
	}
}
