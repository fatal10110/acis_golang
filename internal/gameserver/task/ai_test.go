package task

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameserver/world/worldtest"
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

	state.Spawn(inactive, 0, 0, 0, 0)
	state.Spawn(active, 8192, 0, 0, 0)
	worldtest.SpawnPlayer(state, 3, 8192, 0, 0)
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

func TestAIManagerInactiveRegionResetsOnceAndSleeps(t *testing.T) {
	state := world.New()
	mgr := NewAI(state)
	actor := &aiActorStub{id: 1}

	player := worldtest.SpawnPlayer(state, 2, 0, 0, 0)
	state.Spawn(actor, 0, 0, 0, 0)
	mgr.Add(actor)

	mgr.Tick()

	if actor.ticks != 1 || actor.thinks != 1 {
		t.Fatalf("active actor ticks/thinks = %d/%d, want 1/1", actor.ticks, actor.thinks)
	}

	state.Despawn(player)

	if actor.inactiveCalls != 1 {
		t.Fatalf("inactive calls after deactivation = %d, want 1", actor.inactiveCalls)
	}

	mgr.Tick()
	mgr.Tick()

	if actor.inactiveCalls != 1 {
		t.Fatalf("inactive calls = %d, want 1", actor.inactiveCalls)
	}
	if actor.ticks != 1 || actor.thinks != 1 {
		t.Fatalf("inactive actor ticks/thinks = %d/%d, want unchanged at 1/1", actor.ticks, actor.thinks)
	}

	player = worldtest.SpawnPlayer(state, 2, 0, 0, 0)
	mgr.Tick()

	if actor.ticks != 2 || actor.thinks != 2 {
		t.Fatalf("reactivated actor ticks/thinks = %d/%d, want 2/2", actor.ticks, actor.thinks)
	}

	state.Despawn(player)

	if actor.inactiveCalls != 2 {
		t.Fatalf("inactive calls after second inactive stretch = %d, want 2", actor.inactiveCalls)
	}

	mgr.Tick()

	if actor.inactiveCalls != 2 {
		t.Fatalf("inactive calls after sleeping tick = %d, want 2", actor.inactiveCalls)
	}
}

func TestAIManagerNoSleepInactiveActorKeepsTickingAfterReset(t *testing.T) {
	state := world.New()
	mgr := NewAI(state)
	actor := &aiActorStub{id: 1, keepAwakeInactive: true}

	player := worldtest.SpawnPlayer(state, 2, 0, 0, 0)
	state.Spawn(actor, 0, 0, 0, 0)
	mgr.Add(actor)
	state.Despawn(player)

	if actor.inactiveCalls != 1 {
		t.Fatalf("inactive calls after deactivation = %d, want 1", actor.inactiveCalls)
	}

	mgr.Tick()
	mgr.Tick()

	if actor.inactiveCalls != 1 {
		t.Fatalf("inactive calls = %d, want 1", actor.inactiveCalls)
	}
	if actor.ticks != 2 || actor.thinks != 2 {
		t.Fatalf("no-sleep actor ticks/thinks = %d/%d, want 2/2", actor.ticks, actor.thinks)
	}
}

type aiActorStub struct {
	world.Presence

	mu                sync.Mutex
	id                int32
	ticks             int
	thinks            int
	inactiveCalls     int
	keepAwakeInactive bool
	thinkFn           func()
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

func (a *aiActorStub) OnInactiveRegion() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inactiveCalls++
}

func (a *aiActorStub) SleepWhenRegionInactive() bool { return !a.keepAwakeInactive }
