package persist

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestLaterWriteSupersedesAnEarlierOne(t *testing.T) {
	o := NewOrder()
	first := o.Reserve(1)
	second := o.Reserve(1)

	var landed []string
	second.Run(func([]int32) { landed = append(landed, "second") })
	first.Run(func([]int32) { landed = append(landed, "first") })

	if len(landed) != 1 || landed[0] != "second" {
		t.Fatalf("writes that landed = %v, want only the later one", landed)
	}
}

func TestEarlierWriteStillLandsWhenItRunsFirst(t *testing.T) {
	o := NewOrder()
	first := o.Reserve(1)
	second := o.Reserve(1)

	var landed []string
	first.Run(func([]int32) { landed = append(landed, "first") })
	second.Run(func([]int32) { landed = append(landed, "second") })

	if len(landed) != 2 || landed[0] != "first" || landed[1] != "second" {
		t.Fatalf("writes that landed = %v, want both in order", landed)
	}
}

// Two writes of one row must never run at the same time: that overlap is what
// lets a slower write commit after a faster one and leave the row stale.
func TestWritesOfOneRowDoNotOverlap(t *testing.T) {
	o := NewOrder()
	const writers = 8
	var inFlight, overlaps atomic.Int32
	var wg sync.WaitGroup
	for range writers {
		w := o.Reserve(1)
		wg.Go(func() {
			w.Run(func([]int32) {
				if inFlight.Add(1) != 1 {
					overlaps.Add(1)
				}
				inFlight.Add(-1)
			})
		})
	}
	wg.Wait()
	if got := overlaps.Load(); got != 0 {
		t.Fatalf("overlapping writes of one row = %d, want 0", got)
	}
}

// Different rows are independent: one row's write must not hold another's.
func TestWritesOfDifferentRowsAreIndependent(t *testing.T) {
	o := NewOrder()
	held, other := o.Reserve(1), o.Reserve(2)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		held.Run(func([]int32) { <-release })
	}()

	ran := make(chan struct{})
	go func() { other.Run(func([]int32) { close(ran) }) }()
	<-ran
	close(release)
	<-done
}

// A batch keeps the rows no later write has landed and drops the rest.
func TestBatchWriteKeepsOnlyRowsNoLaterWriteLanded(t *testing.T) {
	o := NewOrder()
	batch := o.Reserve(3, 1, 2)
	later := o.Reserve(2)
	later.Run(func([]int32) {})

	var kept []int32
	batch.Run(func(ids []int32) { kept = append(kept, ids...) })
	if len(kept) != 2 || kept[0] != 1 || kept[1] != 3 {
		t.Fatalf("batch wrote %v, want rows 1 and 3 in id order", kept)
	}
}

// A nil Order is the fixture case: every write runs, with every row.
func TestNilOrderRunsEveryWrite(t *testing.T) {
	var o *Order
	var runs int
	for range 2 {
		o.Reserve(1, 2).Run(func(ids []int32) {
			runs++
			if len(ids) != 2 {
				t.Fatalf("ids = %v, want both rows", ids)
			}
		})
	}
	if runs != 2 {
		t.Fatalf("writes run = %d, want 2", runs)
	}
}

// Reserving a row that has gone quiet must start a fresh place, not compare
// against a stale one.
func TestRowStateIsDroppedOnceQuiet(t *testing.T) {
	o := NewOrder()
	o.Reserve(1).Run(func([]int32) {})
	o.mu.Lock()
	rows := len(o.rows)
	o.mu.Unlock()
	if rows != 0 {
		t.Fatalf("rows still tracked = %d, want 0 once no write is outstanding", rows)
	}
	ran := false
	o.Reserve(1).Run(func([]int32) { ran = true })
	if !ran {
		t.Fatal("a write for a row that went quiet must run")
	}
}

// A lane serves every owner mapped to it, so a write that waits for a row
// another write is holding must not do it on the lane: everything queued
// behind it — other owners' character, shortcut, skill and pet writes — waits
// too. TryRun is what lets such a caller give the lane back.
func TestTryRunGivesUpRatherThanHoldingItsCaller(t *testing.T) {
	o := NewOrder()
	w := New(zerolog.Nop())
	defer w.Close(context.Background())

	// Owner 4 is on lane 0; owners 1 and 5 share lane 1.
	batch := o.Reserve(700, 701)
	started, hold := make(chan struct{}), make(chan struct{})
	w.Enqueue(4, func() {
		batch.Run(func([]int32) {
			close(started)
			<-hold
		})
	})
	<-started

	// A write for one of the held rows, on the other lane.
	contended := o.Reserve(700)
	w.Enqueue(1, func() {
		if contended.TryRun(time.Millisecond, func([]int32) {}) {
			t.Error("took a row the batch is holding")
		}
	})

	// An owner that shares that lane but no row with anyone.
	ran := make(chan struct{})
	w.Enqueue(5, func() { close(ran) })
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		close(hold)
		t.Fatal("an unrelated owner's job stalled behind a row held on another lane")
	}

	close(hold)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	contended.Cancel()
}

// A write that gave up keeps its place: it still lands when it comes back,
// and is still superseded by anything newer that landed meanwhile.
func TestTryRunKeepsItsPlaceForALaterAttempt(t *testing.T) {
	o := NewOrder()
	held := o.Reserve(1)
	waiting := o.Reserve(1)

	taken, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		held.Run(func([]int32) {
			close(taken)
			<-release
		})
	}()
	<-taken

	if waiting.TryRun(time.Millisecond, func([]int32) { t.Error("wrote while the row was held") }) {
		t.Fatal("TryRun reported a write it could not have made")
	}
	close(release)
	<-done

	landed := false
	if !waiting.TryRun(time.Second, func([]int32) { landed = true }) {
		t.Fatal("TryRun gave up on a free row")
	}
	if !landed {
		t.Fatal("the write that came back did not land")
	}
}

// Cancel hands a reservation back without writing, so a row whose write was
// dropped at shutdown does not keep its entry alive.
func TestCancelReleasesWithoutWriting(t *testing.T) {
	o := NewOrder()
	o.Reserve(1).Cancel()
	o.mu.Lock()
	rows := len(o.rows)
	o.mu.Unlock()
	if rows != 0 {
		t.Fatalf("rows still tracked = %d, want 0 after a cancelled reservation", rows)
	}
	landed := false
	o.Reserve(1).Run(func([]int32) { landed = true })
	if !landed {
		t.Fatal("a cancelled reservation must not supersede a later write")
	}
}
