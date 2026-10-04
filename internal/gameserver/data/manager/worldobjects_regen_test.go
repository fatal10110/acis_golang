package manager

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

const regenTestDoorID = 19210010

// newRegenTestWorldObjects spawns one 100 HP door whose regeneration runs
// on a clock only the test moves, starting at the returned time.
func newRegenTestWorldObjects(t *testing.T) (*WorldObjects, *task.DoorRegen, *time.Time, *door.Object) {
	t.Helper()
	objects, _, _, _, _ := newTestWorldObjects(t, &door.Template{ID: regenTestDoorID, Name: "regen_gate", Kind: door.KindDoor, Level: 1})
	now := time.UnixMilli(0)
	regen := task.NewDoorRegen(func() time.Time { return now })
	objects.doorRegen = regen
	gate, ok := objects.Door(regenTestDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	return objects, regen, &now, gate
}

// interleavedRegen runs between before the door's in-flight regeneration
// tick and RegenDoor taking the door lock, the window a concurrent hit on
// the same door can land in.
type interleavedRegen struct {
	w       *WorldObjects
	between func()
}

func (r interleavedRegen) RegenDoor(id int, due time.Time) {
	r.between()
	r.w.RegenDoor(id, due)
}

func TestDoorRegenTickSurvivesHitWhileInFlight(t *testing.T) {
	objects, regen, now, gate := newRegenTestWorldObjects(t)
	start := *now
	objects.ReduceDoorHP(regenTestDoorID, 10) // 90/100, first tick due start+5m

	*now = start.Add(task.DoorRegenPeriod)
	regen.Tick(interleavedRegen{w: objects, between: func() { objects.ReduceDoorHP(regenTestDoorID, 1) }})

	// The hit does not displace the running fixed-rate tick: 90 - 1 + 1.5.
	if got := gate.CurrentHP(); got != 90.5 {
		t.Fatalf("door HP = %v, want 90.5", got)
	}
	if next := start.Add(2 * task.DoorRegenPeriod); !regen.Due(regenTestDoorID, next) {
		t.Fatalf("next tick not due at %v (fixed rate from the first)", next)
	}
}

func TestDoorRegenTickDroppedWhenRescheduledWhileInFlight(t *testing.T) {
	objects, regen, now, gate := newRegenTestWorldObjects(t)
	start := *now
	objects.ReduceDoorHP(regenTestDoorID, 10)

	*now = start.Add(task.DoorRegenPeriod + time.Second)
	regen.Tick(interleavedRegen{w: objects, between: func() {
		// Healed to full (regeneration stops), then hit again (a fresh
		// schedule starts from now).
		objects.doorMu.Lock()
		objects.setDoorHP(gate, float64(gate.MaxHP()), false)
		objects.doorMu.Unlock()
		objects.ReduceDoorHP(regenTestDoorID, 4)
	}})

	if got := gate.CurrentHP(); got != 96 {
		t.Fatalf("door HP = %v, want 96 (stale tick dropped)", got)
	}
	if fresh := now.Add(task.DoorRegenPeriod); !regen.Due(regenTestDoorID, fresh) {
		t.Fatalf("fresh schedule not due at %v", fresh)
	}
}

func TestDoorRegenTickOnFullDoorStopsSchedule(t *testing.T) {
	objects, regen, now, gate := newRegenTestWorldObjects(t)
	regen.Begin(regenTestDoorID)

	*now = now.Add(task.DoorRegenPeriod)
	regen.Tick(objects)

	if got := gate.CurrentHP(); got != float64(gate.MaxHP()) {
		t.Fatalf("door HP = %v, want %d", got, gate.MaxHP())
	}
	if regen.Tracked(regenTestDoorID) {
		t.Fatal("full door still regenerating after its tick")
	}
}
