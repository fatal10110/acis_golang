package task

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

type autosaveFakeActor struct {
	id int32
}

func (a *autosaveFakeActor) ObjectID() int32 { return a.id }

type autosaveFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *autosaveFakeEffects) Save(actor AutosaveActor) {
	e.record(fmt.Sprintf("%d save", actor.ObjectID()))
}

func (e *autosaveFakeEffects) record(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, s)
}

func (e *autosaveFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestAutosaveFiresAfterInitialDelayThenRepeatsAtInterval(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &autosaveFakeEffects{}
	a, err := NewAutosave(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAutosave() error = %v", err)
	}

	actor := &autosaveFakeActor{id: 1}
	a.Add(actor)

	now = now.Add(AutosaveInitialDelay - time.Second)
	a.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before initial delay elapsed = %v, want none", got)
	}

	now = now.Add(time.Second)
	a.Tick()
	if got, want := effects.take(), []string{"1 save"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at initial delay = %v, want %v", got, want)
	}

	now = now.Add(AutosaveInterval - time.Second)
	a.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before interval elapsed = %v, want none", got)
	}

	now = now.Add(time.Second)
	a.Tick()
	if got, want := effects.take(), []string{"1 save"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at next interval = %v, want %v", got, want)
	}
}

func TestAutosaveRemoveStopsSaving(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, func() time.Time { return now })

	actor := &autosaveFakeActor{id: 1}
	a.Add(actor)
	a.Remove(actor.ObjectID())

	now = now.Add(AutosaveInitialDelay)
	a.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after Remove = %v, want none", got)
	}
}

func TestAutosaveRemoveUntrackedActorIsNoop(t *testing.T) {
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, nil)

	a.Remove(1)
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Remove on untracked actor = %v, want none", got)
	}
}

func TestAutosaveAddAlreadyTrackedActorIsNoop(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, func() time.Time { return now })

	actor := &autosaveFakeActor{id: 1}
	a.Add(actor)

	now = now.Add(time.Minute)
	a.Add(actor)

	// The original AutosaveInitialDelay deadline still applies, not one
	// reset by the second Add.
	now = time.UnixMilli(0).Add(AutosaveInitialDelay)
	a.Tick()
	if got, want := effects.take(), []string{"1 save"}; !slices.Equal(got, want) {
		t.Fatalf("Tick after re-Add = %v, want %v", got, want)
	}
}

func TestAutosaveConcurrentAccess(t *testing.T) {
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, nil)
	actors := make([]*autosaveFakeActor, 20)
	for i := range actors {
		actors[i] = &autosaveFakeActor{id: int32(i)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range 200 {
			for _, act := range actors {
				a.Add(act)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			for _, act := range actors {
				a.Remove(act.ObjectID())
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			a.Tick()
		}
	}()
	wg.Wait()
}
