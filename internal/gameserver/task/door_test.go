package task

import (
	"slices"
	"sync"
	"testing"
	"time"
)

type doorFakeEffects struct {
	mu     sync.Mutex
	events []int
}

func (e *doorFakeEffects) ToggleDoor(id int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, id)
}

func (e *doorFakeEffects) take() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestNewDoorRejectsNilEffects(t *testing.T) {
	if _, err := NewDoor(nil, nil); err == nil {
		t.Fatal("NewDoor() error = nil, want error for nil effects")
	}
}

func TestDoorAddThenTickFiresAfterDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &doorFakeEffects{}
	d, err := NewDoor(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDoor() error = %v", err)
	}

	d.Add(19210001, now.Add(7*time.Second))
	if !d.Tracked(19210001) {
		t.Fatal("door should be tracked after Add")
	}

	now = now.Add(6 * time.Second)
	d.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	d.Tick()
	if got, want := effects.take(), []int{19210001}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if d.Tracked(19210001) {
		t.Fatal("door should be removed after toggle fires")
	}
}

func TestDoorCancelStopsPendingToggle(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &doorFakeEffects{}
	d, _ := NewDoor(effects, func() time.Time { return now })

	d.Add(19210001, now.Add(time.Second))

	if !d.Cancel(19210001) {
		t.Fatal("Cancel() = false, want true for tracked door")
	}
	if d.Cancel(19210001) {
		t.Fatal("Cancel() = true, want false for already-removed door")
	}

	now = now.Add(time.Hour)
	d.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after cancel = %v, want none", got)
	}
}

func TestDoorAddReplacesExistingDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &doorFakeEffects{}
	d, _ := NewDoor(effects, func() time.Time { return now })

	d.Add(19210001, now.Add(time.Second))
	d.Add(19210001, now.Add(10*time.Second))

	now = now.Add(time.Second)
	d.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before replaced deadline = %v, want none", got)
	}

	now = now.Add(9 * time.Second)
	d.Tick()
	if got, want := effects.take(), []int{19210001}; !slices.Equal(got, want) {
		t.Fatalf("Tick at replaced deadline = %v, want %v", got, want)
	}
}
