package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type worldObjectIDs struct {
	next int32
}

func (w *worldObjectIDs) NextID() (int32, error) {
	id := w.next
	w.next++
	return id, nil
}

func TestNewWorldObjectsSpawnsDoorsAndStaticObjects(t *testing.T) {
	geo := engine.New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := geo.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	doorX, doorY := engine.WorldX(0), engine.WorldY(0)
	doorTemplates, err := door.NewTable([]*door.Template{{
		ID:       19210001,
		Name:     "test_gate",
		Kind:     door.KindDoor,
		Level:    1,
		Position: location.Location{X: doorX, Y: doorY, Z: 0},
		Coordinates: []location.Point{
			{X: doorX - 8, Y: doorY - 8},
			{X: doorX + 8, Y: doorY - 8},
			{X: doorX + 8, Y: doorY + 8},
			{X: doorX - 8, Y: doorY + 8},
		},
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		Opened: false,
	}})
	if err != nil {
		t.Fatalf("door table: %v", err)
	}

	staticTemplates, err := staticobject.NewTable([]*staticobject.Template{{
		ID:       41001,
		Location: location.Location{X: engine.WorldX(2), Y: engine.WorldY(0), Z: 0},
		Type:     0,
		Texture:  "gludio",
		MapX:     1,
		MapY:     2,
	}})
	if err != nil {
		t.Fatalf("static object table: %v", err)
	}

	state := world.New()
	objects, err := NewWorldObjects(doorTemplates, staticTemplates, &worldObjectIDs{next: 1000}, geo, state)
	if err != nil {
		t.Fatalf("NewWorldObjects: %v", err)
	}

	gate, ok := objects.Door(19210001)
	if !ok {
		t.Fatal("Door(19210001) missing")
	}
	if gate.ObjectID() != 1000 {
		t.Fatalf("door object id = %d, want 1000", gate.ObjectID())
	}
	if gate.Opened() {
		t.Fatal("closed door template spawned opened")
	}
	if got, ok := state.Object(1000); !ok || got != gate {
		t.Fatalf("world object 1000 = %v, %v; want spawned door", got, ok)
	}
	if !gate.Visible() {
		t.Fatal("door is not visible after spawn")
	}
	if x, y, z := gate.Position(); x != doorX || y != doorY || z != 0 {
		t.Fatalf("door position = (%d,%d,%d), want (%d,%d,0)", x, y, z, doorX, doorY)
	}
	if geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0) {
		t.Fatal("closed door did not register a geodata blocker")
	}
	if !objects.SetDoorOpen(19210001, true) {
		t.Fatal("SetDoorOpen(open) = false, want a state change")
	}
	if !geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0) {
		t.Fatal("opened door still blocks geodata movement")
	}
	if !objects.SetDoorOpen(19210001, false) {
		t.Fatal("SetDoorOpen(closed) = false, want a state change")
	}
	if geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0) {
		t.Fatal("reclosed door did not restore its geodata blocker")
	}

	statics := objects.StaticObjects()
	if len(statics) != 1 {
		t.Fatalf("StaticObjects len = %d, want 1", len(statics))
	}
	sign := statics[0]
	if sign.ObjectID() != 1001 || sign.StaticObjectID() != 41001 {
		t.Fatalf("static object ids = object %d static %d, want object 1001 static 41001", sign.ObjectID(), sign.StaticObjectID())
	}
	if got, ok := state.Object(1001); !ok || got != sign {
		t.Fatalf("world object 1001 = %v, %v; want spawned static object", got, ok)
	}
	if !sign.Visible() {
		t.Fatal("static object is not visible after spawn")
	}
}
