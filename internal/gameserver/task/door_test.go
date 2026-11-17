package task

import (
	"slices"
	"sync"
	"testing"
	"time"
)

type doorEvent struct {
	id   int
	open bool
}

type doorFakeEffects struct {
	mu     sync.Mutex
	events []doorEvent
}

func (e *doorFakeEffects) SetDoorOpen(id int, open bool) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, doorEvent{id: id, open: open})
	return true
}

func (e *doorFakeEffects) take() []doorEvent {
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

func TestDoorScheduleThenTickFiresAfterDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &doorFakeEffects{}
	d, err := NewDoor(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDoor() error = %v", err)
	}

	d.Schedule(19210001, false, 7*time.Second)
	if !d.Tracked(19210001) {
		t.Fatal("door should be tracked after Schedule")
	}

	now = now.Add(6 * time.Second)
	d.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	d.Tick()
	if got, want := effects.take(), []doorEvent{{19210001, false}}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if d.Tracked(19210001) {
		t.Fatal("door should be removed after its change fires")
	}
}

func TestDoorCancelDropsEveryPendingChange(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &doorFakeEffects{}
	d, _ := NewDoor(effects, func() time.Time { return now })

	d.Schedule(19210001, false, time.Second)
	d.Schedule(19210001, true, 2*time.Second)
	d.Schedule(19210002, true, 2*time.Second)

	if !d.Cancel(19210001) {
		t.Fatal("Cancel() = false, want true for tracked door")
	}
	if d.Cancel(19210001) {
		t.Fatal("Cancel() = true, want false for already-removed door")
	}

	now = now.Add(time.Hour)
	d.Tick()
	if got, want := effects.take(), []doorEvent{{19210002, true}}; !slices.Equal(got, want) {
		t.Fatalf("Tick after cancel = %v, want %v", got, want)
	}
}

// A later change never replaces one already pending for the door: each
// fires at its own deadline with the target it was scheduled with.
func TestDoorScheduleKeepsEarlierChanges(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &doorFakeEffects{}
	d, _ := NewDoor(effects, func() time.Time { return now })

	d.Schedule(19210001, false, 10*time.Second)
	now = now.Add(3 * time.Second)
	d.Schedule(19210001, false, 10*time.Second)

	now = now.Add(7 * time.Second)
	d.Tick()
	if got, want := effects.take(), []doorEvent{{19210001, false}}; !slices.Equal(got, want) {
		t.Fatalf("Tick at the first deadline = %v, want %v", got, want)
	}
	if !d.Tracked(19210001) {
		t.Fatal("the later change was dropped with the first")
	}

	now = now.Add(3 * time.Second)
	d.Tick()
	if got, want := effects.take(), []doorEvent{{19210001, false}}; !slices.Equal(got, want) {
		t.Fatalf("Tick at the second deadline = %v, want %v", got, want)
	}
}

// Changes falling due in one sweep apply earliest deadline first, and in
// scheduling order between equal deadlines.
func TestDoorTickAppliesDueChangesInDeadlineOrder(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &doorFakeEffects{}
	d, _ := NewDoor(effects, func() time.Time { return now })

	d.Schedule(19210001, false, 900*time.Millisecond)
	d.Schedule(19210001, true, 200*time.Millisecond)
	d.Schedule(19210002, true, 900*time.Millisecond)

	now = now.Add(time.Second)
	d.Tick()
	want := []doorEvent{{19210001, true}, {19210001, false}, {19210002, true}}
	if got := effects.take(); !slices.Equal(got, want) {
		t.Fatalf("Tick = %v, want %v", got, want)
	}
}
