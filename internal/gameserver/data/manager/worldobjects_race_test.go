package manager

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
)

// An unlock opening a door on a player's queue and the door's own timer
// closing it on the timer goroutine must not interleave their state flip
// with the geodata change: the door would end closed with its blocker
// removed (walkable) or open with its blocker in place.
func TestConcurrentDoorOpenAndToggleKeepGeodataInStep(t *testing.T) {
	objects, geo, doorX, doorY, _ := newTestWorldObjects(t, &door.Template{
		ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1,
		OpenTime: 30, CloseTime: 60,
	})
	gate, _ := objects.Door(19210001)

	for round := range 10000 {
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			gate.Open()
		}()
		go func() {
			defer wg.Done()
			<-start
			objects.ToggleDoor(19210001)
		}()
		close(start)
		wg.Wait()

		walkable := geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0)
		if walkable != gate.Opened() {
			t.Fatalf("round %d: door opened=%v but geodata walkable=%v", round, gate.Opened(), walkable)
		}
	}
}
