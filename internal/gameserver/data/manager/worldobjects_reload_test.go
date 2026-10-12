package manager

import (
	"errors"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
)

// A door opened or toggled while the doors reload must land whole on one
// door: never on a door the reload already took out of the world, and never
// between the reload's own state changes, so the geodata always matches the
// state of the door standing.
func TestReloadDoorsWhileTogglingKeepsGeodataInStep(t *testing.T) {
	tmpl := &door.Template{ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1, OpenTime: 30, CloseTime: 60}
	objects, geo, doorX, doorY, _ := newTestWorldObjects(t, tmpl)
	table, err := door.NewTable([]*door.Template{tmpl})
	if err != nil {
		t.Fatalf("door table: %v", err)
	}

	for round := range 300 {
		before, _ := objects.Door(tmpl.ID)
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(3)
		go func() {
			defer wg.Done()
			<-start
			if err := objects.ReloadDoors(table); err != nil {
				t.Errorf("ReloadDoors: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			objects.ToggleDoor(tmpl.ID)
		}()
		go func() {
			defer wg.Done()
			<-start
			before.Open()
		}()
		close(start)
		wg.Wait()

		gate, _ := objects.Door(tmpl.ID)
		if gate == before {
			t.Fatalf("round %d: the reload kept the old door", round)
		}
		if _, ok := objects.state.Object(before.ObjectID()); ok {
			t.Fatalf("round %d: the replaced door is still in the world", round)
		}
		walkable := geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0)
		if walkable != gate.Opened() {
			t.Fatalf("round %d: door opened=%v but geodata walkable=%v", round, gate.Opened(), walkable)
		}
	}
}

// A door the reloaded doors no longer list leaves the world with its
// blocker, auto timer and regeneration.
func TestReloadDoorsDropsAnUnlistedDoor(t *testing.T) {
	tmpl := &door.Template{ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1, OpenTime: 30}
	objects, geo, doorX, doorY, timers := newTestWorldObjects(t, tmpl)
	gate, _ := objects.Door(tmpl.ID)
	objects.doorRegen.Begin(tmpl.ID)
	if !timers.Tracked(tmpl.ID) || geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0) {
		t.Fatal("fixture: the closed gate has no open timer or does not block")
	}
	empty, err := door.NewTable(nil)
	if err != nil {
		t.Fatalf("door table: %v", err)
	}

	if err := objects.ReloadDoors(empty); err != nil {
		t.Fatalf("ReloadDoors: %v", err)
	}
	if _, ok := objects.Door(tmpl.ID); ok || len(objects.Doors()) != 0 {
		t.Fatal("the unlisted door is still spawned")
	}
	if _, ok := objects.state.Object(gate.ObjectID()); ok {
		t.Fatal("the unlisted door is still in the world")
	}
	if timers.Tracked(tmpl.ID) || objects.doorRegen.Tracked(tmpl.ID) {
		t.Fatalf("timer tracked=%v, regeneration tracked=%v after the reload; want neither", timers.Tracked(tmpl.ID), objects.doorRegen.Tracked(tmpl.ID))
	}
	if !geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0) {
		t.Fatal("the unlisted door still blocks geodata")
	}
}

type exhaustedIDs struct{}

func (exhaustedIDs) NextID() (int32, error) { return 0, errors.New("no object id left") }

// A reload whose doors cannot be built closes again the doors it opened
// and keeps them in the world.
func TestReloadDoorsThatCannotBeBuiltKeepsTheDoors(t *testing.T) {
	tmpl := &door.Template{ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1}
	objects, geo, doorX, doorY, _ := newTestWorldObjects(t, tmpl)
	gate, _ := objects.Door(tmpl.ID)
	table, err := door.NewTable([]*door.Template{tmpl})
	if err != nil {
		t.Fatalf("door table: %v", err)
	}
	objects.ids = exhaustedIDs{}

	if err := objects.ReloadDoors(table); err == nil {
		t.Fatal("ReloadDoors with no object id left = nil error")
	}
	if got, _ := objects.Door(tmpl.ID); got != gate || gate.Opened() {
		t.Fatalf("door after the failed reload = %p opened=%v, want the closed boot door %p", got, gate.Opened(), gate)
	}
	if _, ok := objects.state.Object(gate.ObjectID()); !ok {
		t.Fatal("the door left the world")
	}
	if geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0) {
		t.Fatal("the door no longer blocks geodata")
	}
}
