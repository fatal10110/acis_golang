package task

import (
	"sync"
	"testing"
)

func TestPositionUpdatesTickRunsRegisteredMovers(t *testing.T) {
	updates := NewPositionUpdates()
	a := &positionUpdateActorStub{id: 1, moving: true}
	b := &positionUpdateActorStub{id: 2, moving: true}

	updates.Add(a)
	updates.Add(b)
	updates.Tick()

	if a.ticks != 1 || b.ticks != 1 {
		t.Fatalf("ticks = %d/%d, want 1/1", a.ticks, b.ticks)
	}
}

func TestPositionUpdatesTickDropsStoppedMovers(t *testing.T) {
	updates := NewPositionUpdates()
	a := &positionUpdateActorStub{id: 1, moving: false}
	a.remove = func() { updates.Remove(a) }

	updates.Add(a)
	updates.Tick()
	updates.Tick()

	if a.ticks != 1 {
		t.Fatalf("ticks = %d, want 1 after stopped actor is removed", a.ticks)
	}
	if updates.Contains(a) {
		t.Fatal("stopped actor remains registered")
	}
}

func TestPositionUpdatesTickAllowsMutationDuringTick(t *testing.T) {
	updates := NewPositionUpdates()
	a := &positionUpdateActorStub{id: 1, moving: true}
	b := &positionUpdateActorStub{id: 2, moving: true}
	a.tickFn = func() { updates.Remove(b) }

	updates.Add(a)
	updates.Add(b)
	updates.Tick()

	if b.ticks != 1 {
		t.Fatalf("actor b first tick = %d, want 1", b.ticks)
	}

	updates.Tick()
	if b.ticks != 1 {
		t.Fatalf("actor b after removal = %d, want still 1", b.ticks)
	}
}

func TestPositionUpdatesTickAllocationIsFlat(t *testing.T) {
	for _, movers := range []int{1, 128} {
		updates := NewPositionUpdates()
		for i := 0; i < movers; i++ {
			updates.Add(&positionUpdateActorStub{id: int32(i + 1), moving: true})
		}
		updates.Tick()

		allocs := testing.AllocsPerRun(100, updates.Tick)
		if allocs != 0 {
			t.Fatalf("AllocsPerRun(%d movers) = %v, want 0", movers, allocs)
		}
	}
}

func TestPositionUpdatesConcurrentAccess(t *testing.T) {
	updates := NewPositionUpdates()
	actors := make([]*positionUpdateActorStub, 20)
	for i := range actors {
		actors[i] = &positionUpdateActorStub{id: int32(i + 1), moving: true}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, actor := range actors {
				updates.Add(actor)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, actor := range actors {
				updates.Remove(actor)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			updates.Tick()
		}
	}()
	wg.Wait()
}

func BenchmarkPositionUpdatesTickManyMovers(b *testing.B) {
	updates := NewPositionUpdates()
	for i := 0; i < 4096; i++ {
		updates.Add(&positionUpdateActorStub{id: int32(i + 1), moving: true})
	}
	updates.Tick()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updates.Tick()
	}
}

type positionUpdateActorStub struct {
	mu     sync.Mutex
	id     int32
	moving bool
	ticks  int
	tickFn func()
	// remove mirrors Controller's own contract: PositionUpdate deregisters
	// itself when it stops moving instead of relying on Tick to do it.
	remove func()
}

func (a *positionUpdateActorStub) ObjectID() int32 { return a.id }

func (a *positionUpdateActorStub) PositionUpdate() bool {
	a.mu.Lock()
	a.ticks++
	moving := a.moving
	fn := a.tickFn
	remove := a.remove
	a.mu.Unlock()
	if fn != nil {
		fn()
	}
	if !moving && remove != nil {
		remove()
	}
	return moving
}
