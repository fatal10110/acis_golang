package clanhall

import (
	"context"
	"sync"
	"testing"
	"time"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

const (
	feeHall = int32(34)
	feeClan = int32(7)
)

// feeStore is a Store holding the given rows and counting the writes.
type feeStore struct {
	mu      sync.Mutex
	rows    []StoredFunction
	saves   []Function
	deletes int
}

func (s *feeStore) LoadFunctions(context.Context) ([]StoredFunction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]StoredFunction(nil), s.rows...), nil
}

func (s *feeStore) SaveFunction(_ context.Context, _ int32, f Function) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves = append(s.saves, f)
	return nil
}

func (s *feeStore) DeleteFunction(context.Context, int32, int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return nil
}

func (s *feeStore) saved() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.saves)
}

// feeOwners has feeClan own feeHall.
type feeOwners struct{}

func (feeOwners) HallOwner(hallID int32) int32 {
	if hallID == feeHall {
		return feeClan
	}
	return 0
}

// feeTreasury pays every fee and counts them.
type feeTreasury struct {
	mu      sync.Mutex
	paid    []int
	charged chan struct{}
}

func (t *feeTreasury) PayHallFee(clanID int32, adena int) bool {
	if clanID != feeClan {
		return false
	}
	t.mu.Lock()
	t.paid = append(t.paid, adena)
	t.mu.Unlock()
	if t.charged != nil {
		t.charged <- struct{}{}
	}
	return true
}

func (t *feeTreasury) charges() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.paid)
}

// restoredFunctions returns the functions of feeHall, restored from rows.
func restoredFunctions(t *testing.T, store *feeStore) *Functions {
	t.Helper()
	halls, err := hallmodel.NewTable([]*hallmodel.Hall{{ID: int(feeHall), Alias: "moonstone"}})
	if err != nil {
		t.Fatal(err)
	}
	decos, err := hallmodel.NewDecoTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := New(halls, decos, feeOwners{}, store, nil, zerolog.Nop())
	if err := f.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestFunctionWithoutTermChargedOnce: a function whose term has no length
// pays its overdue fee once, stores its row once and is never charged
// again, however long the clock runs.
func TestFunctionWithoutTermChargedOnce(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	store := &feeStore{rows: []StoredFunction{{HallID: feeHall, Function: Function{
		Type: hallmodel.FuncRestoreHP, Level: 80, Lease: 1000, Rate: 0, EndTime: start.UnixMilli() - 1,
	}}}}
	f := restoredFunctions(t, store)
	clock := sim.NewInline(start)
	treasury := &feeTreasury{}
	f.Start(clock.NewQueue("clanhall-functions"), treasury)

	for range 5 {
		clock.Advance(24 * time.Hour)
	}
	if got := treasury.charges(); got != 1 {
		t.Fatalf("fees charged = %d, want 1", got)
	}
	if got := store.saved(); got != 1 {
		t.Fatalf("rows saved = %d, want 1", got)
	}
	if at, armed := clock.NextTimer(); armed {
		t.Fatalf("fee timer still armed for %v, want none", at)
	}
	if got := f.Level(feeHall, hallmodel.FuncRestoreHP); got != 80 {
		t.Fatalf("level after the single charge = %d, want 80 kept", got)
	}
}

// TestReplacedFeeTimerChargesNothing: a fee whose timer was replaced after
// it began to run charges nothing and stores nothing; the replacing timer
// charges once when it comes due.
func TestReplacedFeeTimerChargesNothing(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	day := 24 * time.Hour
	store := &feeStore{rows: []StoredFunction{{HallID: feeHall, Function: Function{
		Type: hallmodel.FuncRestoreMP, Level: 25, Lease: 500, Rate: day.Milliseconds(), EndTime: start.Add(day).UnixMilli(),
	}}}}
	f := restoredFunctions(t, store)
	clock := sim.NewInline(start)
	treasury := &feeTreasury{}
	f.Start(clock.NewQueue("clanhall-functions"), treasury)

	f.mu.Lock()
	fn := f.byHall[feeHall][hallmodel.FuncRestoreMP]
	stale := fn.gen
	f.scheduleLocked(feeHall, fn, 2*day.Milliseconds())
	f.mu.Unlock()

	// The replaced timer's expiry already started: Stop came too late.
	f.payFee(feeHall, fn, stale)
	if got := treasury.charges(); got != 0 {
		t.Fatalf("fees charged by the replaced timer = %d, want 0", got)
	}
	if got := store.saved(); got != 0 {
		t.Fatalf("rows saved by the replaced timer = %d, want 0", got)
	}

	clock.Advance(day)
	if got := treasury.charges(); got != 0 {
		t.Fatalf("fees charged at the replaced deadline = %d, want 0", got)
	}
	clock.Advance(day)
	if got := treasury.charges(); got != 1 {
		t.Fatalf("fees charged at the new deadline = %d, want 1", got)
	}
}

// TestOverdueFeeChargedOnRealClock: on the production clock an overdue fee
// fires at once, racing Start for the timer it arms; it is still charged
// and its next term is armed.
func TestOverdueFeeChargedOnRealClock(t *testing.T) {
	t.Parallel()
	for range 20 {
		store := &feeStore{rows: []StoredFunction{{HallID: feeHall, Function: Function{
			Type: hallmodel.FuncRestoreHP, Level: 80, Lease: 1000, Rate: time.Hour.Milliseconds(), EndTime: 1,
		}}}}
		f := restoredFunctions(t, store)
		pool := sim.NewPool(2, zerolog.Nop())
		pool.Start(context.Background())
		queue := pool.NewQueue("clanhall-functions")
		treasury := &feeTreasury{charged: make(chan struct{}, 1)}
		f.Start(queue, treasury)

		select {
		case <-treasury.charged:
		case <-time.After(5 * time.Second):
			t.Fatal("overdue fee never charged")
		}
		// payFee re-arms under f.mu after the charge; wait for it.
		deadline := time.Now().Add(5 * time.Second)
		for {
			f.mu.Lock()
			armed := f.byHall[feeHall][hallmodel.FuncRestoreHP].timer != nil
			f.mu.Unlock()
			if armed {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("next term never armed after the overdue fee")
			}
			time.Sleep(time.Millisecond)
		}
		queue.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := pool.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		cancel()
	}
}
