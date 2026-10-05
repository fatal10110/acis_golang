package clanhall

import (
	"sync"
	"testing"
	"time"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
)

// gateTreasury blocks inside PayHallFee until released, then answers pay;
// it records every fee asked.
type gateTreasury struct {
	entered chan struct{}
	release chan struct{}
	pay     bool

	mu    sync.Mutex
	asked []int
}

func (t *gateTreasury) PayHallFee(_ int32, adena int) bool {
	t.mu.Lock()
	t.asked = append(t.asked, adena)
	t.mu.Unlock()
	t.entered <- struct{}{}
	<-t.release
	return t.pay
}

func (t *gateTreasury) fees() []int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]int(nil), t.asked...)
}

// TestUpdateDuringFeeKeepsChangedFunction: a function changed while its
// fee is being taken from the clan warehouse keeps the change, whether the
// fee is refused or paid: the change's new term owns the next charge.
func TestUpdateDuringFeeKeepsChangedFunction(t *testing.T) {
	t.Parallel()
	for _, pay := range []bool{false, true} {
		start := time.Unix(1_700_000_000, 0)
		day := 24 * time.Hour
		store := &feeStore{}
		f, clock, _ := startedFunctions(t, store, start)
		treasury := &gateTreasury{entered: make(chan struct{}, 1), release: make(chan struct{}), pay: pay}
		f.mu.Lock()
		f.treasury = treasury
		f.mu.Unlock()

		f.Update(feeHall, hallmodel.FuncRestoreHP, 80, 1000, day.Milliseconds())
		f.mu.Lock()
		fn := f.byHall[feeHall][hallmodel.FuncRestoreHP]
		gen := fn.gen
		f.mu.Unlock()

		// The fee timer's expiry runs (driven directly, as the inline clock
		// would run it on this goroutine); it blocks in the warehouse.
		clock.Advance(day / 2)
		done := make(chan struct{})
		go func() {
			defer close(done)
			f.payFee(feeHall, fn, gen)
		}()
		select {
		case <-treasury.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("fee never reached the warehouse")
		}

		// The leader pays for a change meanwhile.
		f.Update(feeHall, hallmodel.FuncRestoreHP, 140, 1750, day.Milliseconds())
		changedEnd := clock.Now().Add(day).UnixMilli()
		savesBefore := store.saved()
		close(treasury.release)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("fee never finished")
		}

		got, ok := f.Get(feeHall, hallmodel.FuncRestoreHP)
		want := Function{Type: hallmodel.FuncRestoreHP, Level: 140, Lease: 1750, Rate: day.Milliseconds(), EndTime: changedEnd}
		if !ok || got != want {
			t.Fatalf("pay=%v: after the fee, function = %+v, %v; want %+v", pay, got, ok, want)
		}
		if store.deletes != 0 {
			t.Fatalf("pay=%v: rows deleted = %d, want 0", pay, store.deletes)
		}
		if n := store.saved(); n != savesBefore {
			t.Fatalf("pay=%v: rows saved by the stale fee = %d, want 0", pay, n-savesBefore)
		}
		store.mu.Lock()
		last := store.saves[len(store.saves)-1]
		store.mu.Unlock()
		if last != want {
			t.Fatalf("pay=%v: last saved row = %+v, want %+v", pay, last, want)
		}
		// The original deadline passes with nothing charged; the changed
		// term's lease is charged when it ends.
		clock.Advance(time.UnixMilli(changedEnd).Sub(clock.Now()) - time.Millisecond)
		if got := treasury.fees(); len(got) != 1 {
			t.Fatalf("pay=%v: fees before the changed term ends = %v, want [1000]", pay, got)
		}
		clock.Advance(time.Millisecond)
		if got := treasury.fees(); len(got) != 2 || got[1] != 1750 {
			t.Fatalf("pay=%v: fees at the changed term's end = %v, want [1000 1750]", pay, got)
		}
	}
}
