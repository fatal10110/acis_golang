package task

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestAIManagerTickRunsRegisteredActors(t *testing.T) {
	mgr := NewAI(nil)
	a := &aiActorStub{id: 1}
	b := &aiActorStub{id: 2}

	mgr.Add(a)
	mgr.Add(b)
	mgr.Tick()

	if a.ticks != 1 || a.thinks != 1 {
		t.Fatalf("actor a ticks/thinks = %d/%d, want 1/1", a.ticks, a.thinks)
	}
	if b.ticks != 1 || b.thinks != 1 {
		t.Fatalf("actor b ticks/thinks = %d/%d, want 1/1", b.ticks, b.thinks)
	}
}

func TestAIManagerRemoveStopsTicks(t *testing.T) {
	mgr := NewAI(nil)
	a := &aiActorStub{id: 1}

	mgr.Add(a)
	mgr.Remove(a)
	mgr.Tick()

	if a.ticks != 0 || a.thinks != 0 {
		t.Fatalf("ticks/thinks after remove = %d/%d, want 0/0", a.ticks, a.thinks)
	}
}

func TestAIManagerSnapshotAllowsMutationDuringTick(t *testing.T) {
	mgr := NewAI(nil)
	a := &aiActorStub{id: 1}
	b := &aiActorStub{id: 2}
	a.thinkFn = func() { mgr.Remove(b) }

	mgr.Add(a)
	mgr.Add(b)
	mgr.Tick()

	if b.ticks != 1 || b.thinks != 1 {
		t.Fatalf("actor b first tick = %d/%d, want 1/1", b.ticks, b.thinks)
	}

	mgr.Tick()
	if b.ticks != 1 || b.thinks != 1 {
		t.Fatalf("actor b after removal = %d/%d, want still 1/1", b.ticks, b.thinks)
	}
}

func TestAIManagerConcurrentAccess(t *testing.T) {
	mgr := NewAI(nil)
	actors := make([]*aiActorStub, 20)
	for i := range actors {
		actors[i] = &aiActorStub{id: int32(i + 1)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, actor := range actors {
				mgr.Add(actor)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, actor := range actors {
				mgr.Remove(actor)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			mgr.Tick()
		}
	}()
	wg.Wait()
}

func TestAIManagerTickSkipsInactiveRegions(t *testing.T) {
	state := world.New()
	mgr := NewAI(state)
	inactive := &aiActorStub{id: 1}
	active := &aiActorStub{id: 2}
	player := &aiPlayerStub{id: 3}

	state.Spawn(inactive, 0, 0, 0, 0)
	state.Spawn(active, 8192, 0, 0, 0)
	state.Spawn(player, 8192, 0, 0, 0)
	mgr.Add(inactive)
	mgr.Add(active)

	mgr.Tick()

	if inactive.ticks != 0 || inactive.thinks != 0 {
		t.Fatalf("inactive actor ticks/thinks = %d/%d, want 0/0", inactive.ticks, inactive.thinks)
	}
	if active.ticks != 1 || active.thinks != 1 {
		t.Fatalf("active actor ticks/thinks = %d/%d, want 1/1", active.ticks, active.thinks)
	}
}

type aiActorStub struct {
	world.Presence

	mu      sync.Mutex
	id      int32
	ticks   int
	thinks  int
	thinkFn func()
}

func (a *aiActorStub) ObjectID() int32 { return a.id }

func (a *aiActorStub) Tick() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ticks++
}

func (a *aiActorStub) Think() {
	a.mu.Lock()
	a.thinks++
	fn := a.thinkFn
	a.mu.Unlock()
	if fn != nil {
		fn()
	}
}

type aiPlayerStub struct {
	world.Presence

	id int32
}

func (p *aiPlayerStub) ObjectID() int32 { return p.id }

func (p *aiPlayerStub) WorldPlayer() {}
