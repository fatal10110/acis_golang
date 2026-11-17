package persist

import (
	"context"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// laneOwners is one owner id per lane, the service lane's last.
var laneOwners = []int32{0, 1, 2, 3, ServiceOwner}

// Only ServiceOwner reaches the service lane; every other id keeps the lane
// its value modulo the owner lanes gives it, so the ids 0 mod 4 stay on
// lane 0 and share nothing with the game-wide services.
func TestServiceOwnerHasALaneOfItsOwn(t *testing.T) {
	service := LaneIndex(ServiceOwner)
	if service != Lanes-1 {
		t.Fatalf("LaneIndex(ServiceOwner) = %d, want the last lane %d", service, Lanes-1)
	}
	ids := []int32{0, 1, 2, 3, 4, 100, 0x10000000, 0x10000004, math.MaxInt32, -1, -4, math.MinInt32 + 1, math.MinInt32 + 4}
	for i := range int32(1024) {
		ids = append(ids, i, 0x10000000+i)
	}
	for _, id := range ids {
		if got, want := LaneIndex(id), uint32(id)%(Lanes-1); got != want {
			t.Fatalf("LaneIndex(%d) = %d, want %d", id, got, want)
		}
	}
	seen := make(map[uint32]bool)
	for _, id := range laneOwners {
		seen[LaneIndex(id)] = true
	}
	if len(seen) != Lanes {
		t.Fatalf("laneOwners reach %d lanes, want all %d", len(seen), Lanes)
	}
}

// A stalled service write holds neither the jobs nor the flushes of an
// owner whose id is 0 mod 4, and the service jobs still run one at a time in
// the order they were queued once it clears.
func TestServiceLaneHoldsNoOwner(t *testing.T) {
	w := New(zerolog.Nop())
	defer w.Close(context.Background())

	stall := make(chan struct{})
	var mu sync.Mutex
	var order []string
	record := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}
	w.Enqueue(ServiceOwner, func() { <-stall; record("nobles") })
	w.Enqueue(ServiceOwner, func() { record("election") })
	w.Enqueue(ServiceOwner, func() { record("status") })

	for _, owner := range []int32{0, 4, 0x10000000} {
		ran := make(chan struct{})
		w.Enqueue(owner, func() { close(ran) })
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := w.Flush(ctx, owner)
		cancel()
		if err != nil {
			t.Fatalf("Flush(%d) behind a stalled service write: %v", owner, err)
		}
		select {
		case <-ran:
		default:
			t.Fatalf("owner %d's job did not run behind a stalled service write", owner)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := w.Flush(ctx, ServiceOwner); err == nil {
		t.Fatal("Flush(ServiceOwner) returned while its first job was stalled")
	}
	close(stall)
	if err := w.Flush(context.Background(), ServiceOwner); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"nobles", "election", "status"}; !slices.Equal(order, want) {
		t.Fatalf("service jobs ran %q, want %q", order, want)
	}
}
