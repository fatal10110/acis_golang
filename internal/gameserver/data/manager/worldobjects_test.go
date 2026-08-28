package manager

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

type worldObjectIDs struct {
	next int32
}

func (w *worldObjectIDs) NextID() (int32, error) {
	id := w.next
	w.next++
	return id, nil
}

// doorTimerRecorder is a task.DoorEffects that records fired door ids
// instead of scheduling anything, for tests that only need to observe what
// scheduleDoorTimer computed.
type doorTimerRecorder struct{}

func (doorTimerRecorder) ToggleDoor(int) {}

func newDoorGeo(t *testing.T) (*engine.Engine, int, int) {
	t.Helper()
	geo := engine.New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := geo.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	return geo, engine.WorldX(0), engine.WorldY(0)
}

func newTestWorldObjects(t *testing.T, tmpl *door.Template) (*WorldObjects, *engine.Engine, int, int, *task.Door) {
	t.Helper()
	geo, doorX, doorY := newDoorGeo(t)

	tmpl.Position = location.Location{X: doorX, Y: doorY, Z: 0}
	tmpl.Coordinates = []location.Point{
		{X: doorX - 8, Y: doorY - 8},
		{X: doorX + 8, Y: doorY - 8},
		{X: doorX + 8, Y: doorY + 8},
		{X: doorX - 8, Y: doorY + 8},
	}
	tmpl.HP, tmpl.PDef, tmpl.MDef, tmpl.Height = 100, 10, 10, 32

	doorTemplates, err := door.NewTable([]*door.Template{tmpl})
	if err != nil {
		t.Fatalf("door table: %v", err)
	}
	staticTemplates, err := staticobject.NewTable(nil)
	if err != nil {
		t.Fatalf("static object table: %v", err)
	}

	doorTimers, err := task.NewDoor(doorTimerRecorder{}, func() time.Time { return time.UnixMilli(0) })
	if err != nil {
		t.Fatalf("NewDoor: %v", err)
	}

	state := world.New()
	objects, err := NewWorldObjects(doorTemplates, staticTemplates, &worldObjectIDs{next: 1000}, geo, state, doorTimers, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewWorldObjects: %v", err)
	}
	return objects, geo, doorX, doorY, doorTimers
}

func TestNewWorldObjectsSpawnsDoorsAndStaticObjects(t *testing.T) {
	geo, doorX, doorY := newDoorGeo(t)

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

	doorTimers, err := task.NewDoor(doorTimerRecorder{}, nil)
	if err != nil {
		t.Fatalf("NewDoor: %v", err)
	}

	state := world.New()
	objects, err := NewWorldObjects(doorTemplates, staticTemplates, &worldObjectIDs{next: 1000}, geo, state, doorTimers, zerolog.Nop())
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

func TestSpawnDoorWithZeroTimersHasNoScheduledTimer(t *testing.T) {
	_, _, _, _, doorTimers := newTestWorldObjects(t, &door.Template{ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1})

	if doorTimers.Tracked(19210001) {
		t.Fatal("door with openTime=closeTime=0 should have no scheduled timer")
	}
}

func TestSpawnDoorSchedulesInitialCloseTimerForOpenedDoor(t *testing.T) {
	_, _, _, _, doorTimers := newTestWorldObjects(t, &door.Template{
		ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1,
		Opened: true, OpenTime: 30, CloseTime: 60,
	})

	if !doorTimers.Tracked(19210001) {
		t.Fatal("opened door with closeTime>0 should schedule an auto-close timer")
	}
}

func TestSetDoorOpenSchedulesCloseTimerAfterOpening(t *testing.T) {
	objects, _, _, _, doorTimers := newTestWorldObjects(t, &door.Template{
		ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1,
		OpenTime: 30, CloseTime: 60,
	})

	if !objects.SetDoorOpen(19210001, true) {
		t.Fatal("SetDoorOpen(open) = false, want a state change")
	}
	if !doorTimers.Tracked(19210001) {
		t.Fatal("door should have a pending auto-close timer after opening")
	}
}

func TestToggleDoorFlipsStateAndReschedules(t *testing.T) {
	objects, geo, doorX, doorY, doorTimers := newTestWorldObjects(t, &door.Template{
		ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1,
		OpenTime: 30, CloseTime: 60,
	})

	gate, _ := objects.Door(19210001)
	if gate.Opened() {
		t.Fatal("door should spawn closed")
	}

	objects.ToggleDoor(19210001)
	if !gate.Opened() {
		t.Fatal("ToggleDoor should have opened the door")
	}
	if !geo.CanMove(doorX, doorY, 0, engine.WorldX(1), doorY, 0) {
		t.Fatal("opened door still blocks geodata movement")
	}
	if !doorTimers.Tracked(19210001) {
		t.Fatal("door should have a pending auto-close timer after ToggleDoor opened it")
	}

	objects.ToggleDoor(19210001)
	if gate.Opened() {
		t.Fatal("ToggleDoor should have closed the door back")
	}
}

func TestScheduleDoorTimerCancelsPendingWhenDelayBecomesZero(t *testing.T) {
	objects, _, _, _, doorTimers := newTestWorldObjects(t, &door.Template{
		ID: 19210001, Name: "gate", Kind: door.KindDoor, Level: 1,
		OpenTime: 30, CloseTime: 0,
	})

	if !objects.SetDoorOpen(19210001, true) {
		t.Fatal("SetDoorOpen(open) = false, want a state change")
	}
	// closeTime is 0, so opening the door should leave no pending timer.
	if doorTimers.Tracked(19210001) {
		t.Fatal("door should have no pending timer when closeTime=0")
	}
}

// TestNewWorldObjectsSkipsDegenerateDoor matches DoorData.java:113-123,
// which logs and skips a door whose footprint fails to triangulate rather
// than aborting the whole load (issue #1901).
func TestNewWorldObjectsSkipsDegenerateDoor(t *testing.T) {
	geo, x, y := newDoorGeo(t)
	valid := &door.Template{
		ID:       1,
		Name:     "valid",
		Kind:     door.KindDoor,
		Position: location.Location{X: x, Y: y, Z: 0},
		Coordinates: []location.Point{
			{X: x - 16, Y: y - 16},
			{X: x - 16, Y: y + 16},
			{X: x + 16, Y: y + 16},
			{X: x + 16, Y: y - 16},
		},
	}
	degenerate := &door.Template{
		ID:       2,
		Name:     "degenerate",
		Kind:     door.KindDoor,
		Position: location.Location{X: x, Y: y, Z: 0},
		Coordinates: []location.Point{
			{X: x, Y: y},
			{X: x + 1, Y: y},
		},
	}
	doors, err := door.NewTable([]*door.Template{valid, degenerate})
	if err != nil {
		t.Fatalf("door.NewTable(): %v", err)
	}
	statics, err := staticobject.NewTable(nil)
	if err != nil {
		t.Fatalf("staticobject.NewTable(): %v", err)
	}
	doorTimers, err := task.NewDoor(doorTimerRecorder{}, nil)
	if err != nil {
		t.Fatalf("NewDoor: %v", err)
	}

	var logs bytes.Buffer
	objs, err := NewWorldObjects(doors, statics, &worldObjectIDs{}, geo, world.New(), doorTimers, zerolog.New(&logs))
	if err != nil {
		t.Fatalf("NewWorldObjects() error: %v", err)
	}

	got := objs.Doors()
	if len(got) != 1 {
		t.Fatalf("Doors() len = %d, want 1", len(got))
	}
	if got[0].DoorID() != 1 {
		t.Fatalf("Doors()[0].DoorID() = %d, want 1", got[0].DoorID())
	}
	if !strings.Contains(logs.String(), "degenerate footprint") || !strings.Contains(logs.String(), `"door":2`) {
		t.Fatalf("log = %q, want degenerate-footprint diagnostic for door 2", logs.String())
	}
}

// TestSetDoorOpenCascadesToTriggeredDoor matches Door.changeState's
// triggerId propagation (aCis_gameserver Door.java:391-398): opening or
// closing a controller door applies the same state to its Template.TriggeredID
// door, without scheduling that linked door's own auto-timer (issue #2014).
func TestSetDoorOpenCascadesToTriggeredDoor(t *testing.T) {
	geo, x, y := newDoorGeo(t)
	coords := []location.Point{
		{X: x - 8, Y: y - 8},
		{X: x + 8, Y: y - 8},
		{X: x + 8, Y: y + 8},
		{X: x - 8, Y: y + 8},
	}
	controller := &door.Template{
		ID: 19210001, Name: "controller", Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: x, Y: y, Z: 0}, Coordinates: coords,
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		TriggeredID: 19210002,
	}
	linked := &door.Template{
		ID: 19210002, Name: "linked", Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: x, Y: y, Z: 0}, Coordinates: coords,
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		CloseTime: 60,
	}
	doorTemplates, err := door.NewTable([]*door.Template{controller, linked})
	if err != nil {
		t.Fatalf("door table: %v", err)
	}
	staticTemplates, err := staticobject.NewTable(nil)
	if err != nil {
		t.Fatalf("static object table: %v", err)
	}
	doorTimers, err := task.NewDoor(doorTimerRecorder{}, nil)
	if err != nil {
		t.Fatalf("NewDoor: %v", err)
	}

	objects, err := NewWorldObjects(doorTemplates, staticTemplates, &worldObjectIDs{next: 1000}, geo, world.New(), doorTimers, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewWorldObjects: %v", err)
	}

	if !objects.SetDoorOpen(19210001, true) {
		t.Fatal("SetDoorOpen(controller, open) = false, want a state change")
	}
	linkedObj, _ := objects.Door(19210002)
	if !linkedObj.Opened() {
		t.Fatal("triggered door did not follow the controller door's open state")
	}
	if doorTimers.Tracked(19210002) {
		t.Fatal("cascaded state change must not schedule the triggered door's own auto-timer")
	}

	if !objects.SetDoorOpen(19210001, false) {
		t.Fatal("SetDoorOpen(controller, close) = false, want a state change")
	}
	if linkedObj.Opened() {
		t.Fatal("triggered door did not follow the controller door's close state")
	}
}
