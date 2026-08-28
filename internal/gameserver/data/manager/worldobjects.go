package manager

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// WorldObjects owns the always-spawned doors and static objects loaded at boot.
type WorldObjects struct {
	geo        *engine.Engine
	state      *world.State
	doorTimers *task.Door
	now        func() time.Time

	doors       map[int]*door.Object
	doorOrder   []*door.Object
	staticOrder []*staticobject.Object
}

// NewWorldObjects allocates, spawns, and indexes door and static-object
// templates. Closed doors are applied to geodata immediately. doorTimers
// schedules each door's next auto open/close transition, mirroring the
// reference server's DoorAI. A door whose triangulated footprint is
// degenerate or samples to no geodata cells is logged and skipped rather
// than aborting boot, matching DoorData.java:113-123.
func NewWorldObjects(doors *door.Table, statics *staticobject.Table, ids idAllocator, geo *engine.Engine, state *world.State, doorTimers *task.Door, log zerolog.Logger) (*WorldObjects, error) {
	if ids == nil {
		return nil, fmt.Errorf("world objects: nil id allocator")
	}
	if geo == nil {
		return nil, fmt.Errorf("world objects: nil geo engine")
	}
	if state == nil {
		return nil, fmt.Errorf("world objects: nil world state")
	}
	if doorTimers == nil {
		return nil, fmt.Errorf("world objects: nil door timers")
	}

	w := &WorldObjects{
		geo:        geo,
		state:      state,
		doorTimers: doorTimers,
		now:        time.Now,
		doors:      make(map[int]*door.Object),
	}
	for _, tmpl := range doors.All() {
		obj, err := w.spawnDoor(tmpl, ids)
		if err != nil {
			if errors.Is(err, door.ErrEmptyFootprint) || errors.Is(err, dynamic.ErrDegenerateFootprint) {
				log.Warn().Err(err).Int("door", tmpl.ID).Msg("data/manager: skipping door with degenerate footprint")
				continue
			}
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

// SetDoorOpen changes a door's open state, applies the matching geodata,
// broadcasts the change to known observers, reschedules the door's next auto
// open/close timer from its template's openTime/closeTime/randomTime, and
// propagates the same state to a linked controller door (Template.TriggeredID),
// mirroring the reference server's Door.changeState(open, false).
func (w *WorldObjects) SetDoorOpen(id int, open bool) bool {
	obj, ok := w.Door(id)
	if !ok {
		return false
	}
	return w.changeDoorState(obj, open, false)
}

// changeDoorState mirrors Door.changeState(open, triggered): triggered is
// true only for a cascaded change propagated from another door's
// Template.TriggeredID, and suppresses this door's own auto-timer reschedule
// so the linked door's cascade doesn't double-schedule it.
func (w *WorldObjects) changeDoorState(obj *door.Object, open, triggered bool) bool {
	if !obj.SetOpened(open) {
		return false
	}
	if open {
		w.geo.RemoveObject(obj)
	} else {
		w.geo.AddObject(obj)
	}
	obj.BroadcastStatus()
	if obj.Template.TriggeredID > 0 {
		if linked, ok := w.Door(obj.Template.TriggeredID); ok {
			w.changeDoorState(linked, open, true)
		}
	}
	if !triggered {
		w.scheduleDoorTimer(obj)
	}
	return true
}

// ToggleDoor implements task.DoorEffects: it flips id's door to the opposite
// of its current state once a scheduled timer fires.
func (w *WorldObjects) ToggleDoor(id int) {
	obj, ok := w.Door(id)
	if !ok {
		return
	}
	w.SetDoorOpen(id, !obj.Opened())
}

// scheduleDoorTimer schedules obj's next auto transition: closeTime after an
// open, openTime after a close, jittered by a uniform [0, randomTime) delay.
// A total delay of zero or less leaves the door with no pending timer.
func (w *WorldObjects) scheduleDoorTimer(obj *door.Object) {
	tmpl := obj.Template
	delay := tmpl.CloseTime
	if !obj.Opened() {
		delay = tmpl.OpenTime
	}
	if tmpl.RandomTime > 0 {
		delay += rnd.Get(tmpl.RandomTime)
	}
	if delay <= 0 {
		w.doorTimers.Cancel(obj.DoorID())
		return
	}
	w.doorTimers.Add(obj.DoorID(), w.now().Add(time.Duration(delay)*time.Second))
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
	obj.SetWorld(w.state)
	obj.SetFrameBuilder(serverpackets.DoorFrameBuilder{})
	w.state.Spawn(obj, tmpl.Position.X, tmpl.Position.Y, tmpl.Position.Z, 0)
	if !obj.Opened() {
		w.geo.AddObject(obj)
	}
	w.scheduleDoorTimer(obj)
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
