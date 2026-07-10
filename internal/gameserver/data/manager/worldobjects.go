package manager

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// WorldObjects owns the always-spawned doors and static objects loaded at boot.
type WorldObjects struct {
	geo   *engine.Engine
	state *world.State

	doors       map[int]*door.Object
	doorOrder   []*door.Object
	staticOrder []*staticobject.Object
}

// NewWorldObjects allocates, spawns, and indexes door and static-object
// templates. Closed doors are applied to geodata immediately.
func NewWorldObjects(doors *door.Table, statics *staticobject.Table, ids idAllocator, geo *engine.Engine, state *world.State) (*WorldObjects, error) {
	if ids == nil {
		return nil, fmt.Errorf("world objects: nil id allocator")
	}
	if geo == nil {
		return nil, fmt.Errorf("world objects: nil geo engine")
	}
	if state == nil {
		return nil, fmt.Errorf("world objects: nil world state")
	}

	w := &WorldObjects{
		geo:   geo,
		state: state,
		doors: make(map[int]*door.Object),
	}
	for _, tmpl := range doors.All() {
		obj, err := w.spawnDoor(tmpl, ids)
		if err != nil {
			return nil, err
		}
		w.doors[obj.DoorID()] = obj
		w.doorOrder = append(w.doorOrder, obj)
	}
	for _, tmpl := range statics.All() {
		obj, err := w.spawnStaticObject(tmpl, ids)
		if err != nil {
			return nil, err
		}
		w.staticOrder = append(w.staticOrder, obj)
	}
	return w, nil
}

// Door returns the spawned door for id.
func (w *WorldObjects) Door(id int) (*door.Object, bool) {
	if w == nil {
		return nil, false
	}
	obj, ok := w.doors[id]
	return obj, ok
}

// Doors returns spawned doors in template order.
func (w *WorldObjects) Doors() []*door.Object {
	if w == nil {
		return nil
	}
	return append([]*door.Object(nil), w.doorOrder...)
}

// StaticObjects returns spawned static objects in template order.
func (w *WorldObjects) StaticObjects() []*staticobject.Object {
	if w == nil {
		return nil
	}
	return append([]*staticobject.Object(nil), w.staticOrder...)
}

// SetDoorOpen changes a door's open state and applies the matching geodata.
func (w *WorldObjects) SetDoorOpen(id int, open bool) bool {
	obj, ok := w.Door(id)
	if !ok || !obj.SetOpened(open) {
		return false
	}
	if open {
		w.geo.RemoveObject(obj)
	} else {
		w.geo.AddObject(obj)
	}
	return true
}

func (w *WorldObjects) spawnDoor(tmpl *door.Template, ids idAllocator) (*door.Object, error) {
	id, err := ids.NextID()
	if err != nil {
		return nil, fmt.Errorf("world objects: door %d: %w", tmpl.ID, err)
	}
	shape, err := dynamic.NewDoorObject(tmpl, w.geo)
	if err != nil {
		return nil, fmt.Errorf("world objects: door %d: %w", tmpl.ID, err)
	}
	obj, err := door.NewObject(id, tmpl, shape)
	if err != nil {
		return nil, fmt.Errorf("world objects: door %d: %w", tmpl.ID, err)
	}
	w.state.Spawn(obj, tmpl.Position.X, tmpl.Position.Y, tmpl.Position.Z, 0)
	if !obj.Opened() {
		w.geo.AddObject(obj)
	}
	return obj, nil
}

func (w *WorldObjects) spawnStaticObject(tmpl *staticobject.Template, ids idAllocator) (*staticobject.Object, error) {
	id, err := ids.NextID()
	if err != nil {
		return nil, fmt.Errorf("world objects: static object %d: %w", tmpl.ID, err)
	}
	obj, err := staticobject.NewObject(id, tmpl)
	if err != nil {
		return nil, fmt.Errorf("world objects: static object %d: %w", tmpl.ID, err)
	}
	w.state.Spawn(obj, tmpl.Location.X, tmpl.Location.Y, tmpl.Location.Z, 0)
	return obj, nil
}
