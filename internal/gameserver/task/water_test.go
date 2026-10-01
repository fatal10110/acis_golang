package task

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// ---- from water_test.go ----
type waterFakeActor struct {
	id    int32
	queue *sim.Queue
	dead  bool
}

func (a *waterFakeActor) ObjectID() int32 { return a.id }

func (a *waterFakeActor) Dead() bool { return a.dead }

type waterFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *waterFakeEffects) GaugeSet(actor WaterActor, remaining time.Duration) {
	e.record(fmt.Sprintf("%d gauge %s", actor.ObjectID(), remaining))
}

func (e *waterFakeEffects) Drown(actor WaterActor) {
	e.record(fmt.Sprintf("%d drown", actor.ObjectID()))
}

func (e *waterFakeEffects) record(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, s)
}

func (e *waterFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestWaterAddStartsCountdownAndDrownsAfterBreathElapses(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &waterFakeEffects{}
	w, err := NewWater(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}

	actor := &waterFakeActor{id: 1}
	w.Add(actor, 10*time.Second)
	if got, want := effects.take(), []string{"1 gauge 10s"}; !slices.Equal(got, want) {
		t.Fatalf("after Add = %v, want %v", got, want)
	}

	now = now.Add(9 * time.Second)
	w.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before breath elapsed = %v, want none", got)
	}

	now = now.Add(time.Second)
	w.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"1 drown"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at breath limit = %v, want %v", got, want)
	}

	// Drowning repeats every tick until the actor is removed.
	now = now.Add(time.Second)
	w.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"1 drown"}; !slices.Equal(got, want) {
		t.Fatalf("Tick after breath limit = %v, want %v", got, want)
	}
}

func TestWaterRemoveStopsDrowning(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, func() time.Time { return now })

	actor := &waterFakeActor{id: 1}
	w.Add(actor, time.Second)
	effects.take()

	now = now.Add(2 * time.Second)
	w.Remove(actor)
	if got, want := effects.take(), []string{"1 gauge 0s"}; !slices.Equal(got, want) {
		t.Fatalf("after Remove = %v, want %v", got, want)
	}

	w.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after Remove = %v, want none", got)
	}
}

func TestWaterRemoveUntrackedActorIsNoop(t *testing.T) {
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, nil)

	w.Remove(&waterFakeActor{id: 1})
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Remove on untracked actor = %v, want none", got)
	}
}

func TestWaterAddDeadActorIsNoop(t *testing.T) {
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, nil)

	w.Add(&waterFakeActor{id: 1, dead: true}, time.Second)
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Add on dead actor = %v, want none", got)
	}
}

func TestWaterAddAlreadyTrackedActorIsNoop(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, func() time.Time { return now })

	actor := &waterFakeActor{id: 1}
	w.Add(actor, time.Second)
	effects.take()

	w.Add(actor, 5*time.Second)
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("second Add on tracked actor = %v, want none", got)
	}

	// The original one-second deadline still applies, not the second call's.
	now = now.Add(time.Second)
	w.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"1 drown"}; !slices.Equal(got, want) {
		t.Fatalf("Tick after re-Add = %v, want %v", got, want)
	}
}

func TestWaterConcurrentAccess(t *testing.T) {
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, nil)
	actors := make([]*waterFakeActor, 20)
	for i := range actors {
		actors[i] = &waterFakeActor{id: int32(i)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, a := range actors {
				w.Add(a, time.Millisecond)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, a := range actors {
				w.Remove(a)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			w.Tick()
			testLoop.Run()
		}
	}()
	wg.Wait()
}

func (waterFakeActor) Kind() actor.Kind { return actor.KindNPC }

func (a *waterFakeActor) Queue() *sim.Queue { return cmp.Or(a.queue, testQueue) }

// TestWaterExitBeforeQueuedDrownSkipsDamage covers the same ordering for
// drowning: leaving the water ahead of the sweep's queued damage must keep
// that damage from landing on a surfaced player.
func TestWaterExitBeforeQueuedDrownSkipsDamage(t *testing.T) {
	inline := sim.NewInline(time.UnixMilli(0))
	base := time.UnixMilli(0)
	now := base
	effects := &waterFakeEffects{}
	water, err := NewWater(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}
	actor := &waterFakeActor{id: 1, queue: inline.NewQueue("actor-1")}
	water.Add(actor, time.Second)

	now = base.Add(2 * time.Second)
	// Surfacing lands on the queue ahead of the sweep's drowning damage.
	actor.queue.Post(func() { water.Remove(actor) })
	water.Tick()
	inline.Run()

	for _, event := range effects.take() {
		if strings.Contains(event, "drown") {
			t.Fatal("drowning damage landed after the player left the water")
		}
	}

	// A fresh submersion still drowns once its own breath runs out.
	water.Add(actor, time.Second)
	now = now.Add(2 * time.Second)
	water.Tick()
	inline.Run()
	events := effects.take()
	drowns := 0
	for _, event := range events {
		if strings.Contains(event, "drown") {
			drowns++
		}
	}
	if drowns != 1 {
		t.Fatalf("drowning damage after a fresh breath = %d, want 1: %v", drowns, events)
	}
}
