package olympiad

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// gatedStore records each write and holds every one until release is
// closed, as a stalled database would.
type gatedStore struct {
	*gamesql.OlympiadStore
	release chan struct{}
	mu      sync.Mutex
	writes  []string
}

func (s *gatedStore) wait(format string, args ...any) {
	<-s.release
	s.mu.Lock()
	s.writes = append(s.writes, fmt.Sprintf(format, args...))
	s.mu.Unlock()
}

func (s *gatedStore) written() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.writes)
}

func (s *gatedStore) SaveCycle(ctx context.Context, cycle int32) error {
	s.wait("save_cycle %d", cycle)
	return s.OlympiadStore.SaveCycle(ctx, cycle)
}

func (s *gatedStore) SaveNobles(ctx context.Context, nobles map[int32]olympiad.Noble) error {
	s.wait("save_nobles %s", nobleList(nobles))
	return s.OlympiadStore.SaveNobles(ctx, nobles)
}

func (s *gatedStore) DeleteNobles(ctx context.Context) error {
	s.wait("delete_nobles")
	return s.OlympiadStore.DeleteNobles(ctx)
}

func (s *gatedStore) SnapshotMonth(ctx context.Context) error {
	s.wait("snapshot_month")
	return s.OlympiadStore.SnapshotMonth(ctx)
}

// TestOlympiadStepsDoNotWaitOnTheDatabase pins that the calendar's
// database writes run on the persistence lane, not on its queue: with
// every write stalled, the competition end and the next day's new cycle
// still run and change the held state at once; the writes land later in
// the order the steps made them, the truncate before the save that
// follows it; and Stop waits for all of them and its own save.
func TestOlympiadStepsDoNotWaitOnTheDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 2, map[int32]int{1001: 5})
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() { _ = worker.Close(context.Background()) })
	store := &gatedStore{OlympiadStore: gamesql.NewOlympiadStore(db), release: make(chan struct{})}
	loop := sim.NewInline(time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC))
	o := olympiad.New(olympiad.DefaultConfig(), store, worker, discard{}, loop.NewQueue("olympiad"), zerolog.Nop())
	if err := o.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	o.Start()
	loop.Run()

	// The competition end at midnight queues the cycle and records save;
	// the validation end at 18:00 opens cycle 3 and queues the truncate.
	stepped := make(chan struct{})
	go func() {
		loop.Advance(22 * time.Hour)
		close(stepped)
	}()
	select {
	case <-stepped:
	case <-time.After(5 * time.Second):
		close(store.release)
		t.Fatal("a calendar step waited on a stalled database write")
	}
	if got := o.Cycle(); got != 3 {
		t.Fatalf("cycle = %d, want 3", got)
	}
	if _, ok := o.Noble(1001); ok {
		t.Fatal("the new cycle kept the held record")
	}
	if got := store.written(); len(got) != 0 {
		t.Fatalf("writes landed while the database was stalled: %q", got)
	}

	stopped := make(chan struct{})
	go func() {
		o.Stop(ctx)
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned before the queued writes landed")
	case <-time.After(100 * time.Millisecond):
	}
	close(store.release)
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not return once the writes landed")
	}

	want := []string{"save_cycle 2", "save_nobles 1001:5", "delete_nobles", "save_cycle 3"}
	if got := store.written(); !slices.Equal(got, want) {
		t.Fatalf("writes = %q, want %q", got, want)
	}
	var rows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM olympiad_nobles").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("olympiad_nobles rows = %d, %v; want 0", rows, err)
	}
	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM server_memo WHERE var = 'olympiad_cycle'").Scan(&value); err != nil || value != "3" {
		t.Fatalf("stored cycle = %q, %v; want \"3\"", value, err)
	}
}
