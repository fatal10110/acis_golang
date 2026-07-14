package task

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

type decayFakeActor struct {
	id int32
}

func (a *decayFakeActor) ObjectID() int32 { return a.id }

type decayFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *decayFakeEffects) Decay(actor DecayActor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, fmt.Sprintf("decay %d", actor.ObjectID()))
}

func (e *decayFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestNewDecayRejectsNilEffects(t *testing.T) {
	if _, err := NewDecay(nil, nil); err == nil {
		t.Fatal("NewDecay() error = nil, want error for nil effects")
	}
}

func TestDecayAddThenTickFiresAfterDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, 7*time.Second)
	if !decay.Tracked(actor) {
		t.Fatal("actor should be tracked after Add")
	}

	now = now.Add(6 * time.Second)
	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	decay.Tick()
	if got, want := effects.take(), []string{"decay 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if decay.Tracked(actor) {
		t.Fatal("actor should be removed after decay fires")
	}
}

func TestDecayDeadlineReportsTrackedDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	if _, ok := decay.Deadline(actor); ok {
		t.Fatal("Deadline() ok = true before Add, want false")
	}

	decay.Add(actor, 7*time.Second)
	if got, ok := decay.Deadline(actor); !ok || !got.Equal(now.Add(7*time.Second)) {
		t.Fatalf("Deadline() = %v, %v; want %v, true", got, ok, now.Add(7*time.Second))
	}

	decay.Cancel(actor)
	if _, ok := decay.Deadline(actor); ok {
		t.Fatal("Deadline() ok = true after Cancel, want false")
	}
}

func TestDecayCancelStopsPendingDecay(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, time.Second)

	if !decay.Cancel(actor) {
		t.Fatal("Cancel() = false, want true for tracked actor")
	}
	if decay.Cancel(actor) {
		t.Fatal("Cancel() = true, want false for already-removed actor")
	}

	now = now.Add(time.Hour)
	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after cancel = %v, want none", got)
	}
}

func TestDecayAddReplacesExistingDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, time.Second)
	decay.Add(actor, 10*time.Second)

	now = now.Add(time.Second)
	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before replaced deadline = %v, want none", got)
	}

	now = now.Add(9 * time.Second)
	decay.Tick()
	if got, want := effects.take(), []string{"decay 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at replaced deadline = %v, want %v", got, want)
	}
}

func TestDecayConcurrentAddAndTick(t *testing.T) {
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			actor := &decayFakeActor{id: id}
			decay.Add(actor, 0)
			decay.Tick()
			decay.Cancel(actor)
		}(int32(i))
	}
	wg.Wait()
}
