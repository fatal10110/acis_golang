package network

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// TestFlushWaitsForARequeuedItemWrite pins what a lane flush means once a
// write can hand the lane back. awaitPersistence waits for a lane before a
// restart reads the items table, so a flush that returned while a write it
// queued was still going round would let the client's next session restore
// from rows that write has not reached.
//
// The row is held by a stand-in for the persistence tick's chunk, so the
// write's first attempt has to give up; the flush marker is pushed after the
// write was queued, which is the order that broke.
func TestFlushWaitsForARequeuedItemWrite(t *testing.T) {
	order := persist.NewOrder()
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() {
		if err := worker.Close(context.Background()); err != nil {
			t.Errorf("close persistence worker: %v", err)
		}
	})
	link := &GameClientLink{persist: worker, itemWrites: order, log: zerolog.Nop()}

	const ownerID, objectID int32 = 1, 900
	held, release := make(chan struct{}), make(chan struct{})
	holder := order.Reserve(objectID)
	// Owner 4 shares no lane with owner 1, so only the row is contended.
	worker.Enqueue(4, func() {
		holder.Run(func([]int32) {
			close(held)
			<-release
		})
	})
	<-held

	var wrote atomic.Bool
	link.queueItemWrite(ownerID, order.Reserve(objectID), func() { wrote.Store(true) })

	flushed := make(chan error, 1)
	go func() { flushed <- worker.Flush(context.Background(), ownerID) }()
	select {
	case err := <-flushed:
		close(release)
		t.Fatalf("flush returned (%v) while the item write it was meant to wait for was still queued", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-flushed:
		if err != nil {
			t.Fatalf("flush: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("flush never returned after the row was released")
	}
	if !wrote.Load() {
		t.Fatal("flush returned before the item write landed")
	}
}
