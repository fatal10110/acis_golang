package manager

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// WorldObjects owns the always-spawned doors and static objects loaded at boot.
type WorldObjects struct {
	geo        *engine.Engine
	state      *world.State
	doorTimers *task.Door
	doorRegen  *task.DoorRegen
	newSink    func(*door.Object) event.Sink
	now        func() time.Time

	// doorMu serializes door state changes, so a flag flip or an HP change
	// and its geodata, broadcast and timer follow-up land as one step
	// whichever goroutine (the door timer, the regeneration sweep, a
	// player's unlock or hit) makes it.
	doorMu sync.Mutex

	doors       map[int]*door.Object
	doorOrder   []*door.Object
	staticOrder []*staticobject.Object
}

// NewWorldObjects allocates, spawns, and indexes door and static-object
// templates. Closed doors are applied to geodata immediately. doorTimers
// schedules each door's next auto open/close transition, and doorRegen each
// damaged door's regeneration ticks, which RegenDoor applies. A door whose
// triangulated footprint is degenerate or samples to no geodata cells is
// logged and skipped rather than aborting boot.
func NewWorldObjects(doors *door.Table, statics *staticobject.Table, ids idAllocator, geo *engine.Engine, state *world.State, doorTimers *task.Door, doorRegen *task.DoorRegen, newSink func(*door.Object) event.Sink, log zerolog.Logger) (*WorldObjects, error) {
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
	if doorRegen == nil {
		return nil, fmt.Errorf("world objects: nil door regeneration")
	}
	if newSink == nil {
		// Doors built without a sink factory never reach a client: no
		// open/close state change is broadcast and nothing errors. Domain
		// tests boot this way on purpose, so this warns rather than fails,
		// but a production composition root reaching it is a wiring bug.
		log.Warn().Msg("data/manager: no door event sink factory; doors will not broadcast state changes")
	}

	w := &WorldObjects{
		geo:        geo,
		state:      state,
		doorTimers: doorTimers,
		doorRegen:  doorRegen,
		newSink:    newSink,
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
// propagates the same state to a linked controller door (Template.TriggeredID).
func (w *WorldObjects) SetDoorOpen(id int, open bool) bool {
	obj, ok := w.Door(id)
	if !ok {
		return false
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	return w.changeDoorState(obj, open, false)
}

// changeDoorState mirrors Door.changeState(open, triggered); the caller
// holds doorMu. A broken door keeps its state. triggered is true only for a
// cascaded change propagated from another door's Template.TriggeredID, and
// suppresses this door's own auto-timer reschedule so the linked door's
// cascade doesn't double-schedule it.
func (w *WorldObjects) changeDoorState(obj *door.Object, open, triggered bool) bool {
	if obj.Dead() || !obj.SetOpened(open) {
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
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	w.changeDoorState(obj, !obj.Opened(), false)
}

// ReduceDoorHP implements door.StateOwner: it takes amount off id's door HP
// once door.Object.ReduceHP let the hit through, broadcasting the new HP,
// and breaks the door once less than half a point is left. A broken door
// takes no more damage.
func (w *WorldObjects) ReduceDoorHP(id int, amount float64) {
	obj, ok := w.Door(id)
	if !ok {
		return
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	if obj.Dead() {
		return
	}
	if amount > 0 {
		w.setDoorHP(obj, math.Max(obj.CurrentHP()-amount, 0), true)
	}
	if obj.CurrentHP() < 0.5 {
		w.breakDoor(obj)
	}
}

// ReviveDoor stands id's broken door again, as a siege's door respawn does:
// it takes its template's open state back, blocking geodata again when that
// state is closed, regains restoreHP of its maximum HP, and observers see
// the HP and then the revive. It reports whether the door was broken.
func (w *WorldObjects) ReviveDoor(id int, restoreHP float64) bool {
	obj, ok := w.Door(id)
	if !ok {
		return false
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	if !obj.Dead() {
		return false
	}
	obj.SetOpened(obj.Template.Opened)
	if !obj.Opened() {
		w.geo.AddObject(obj)
	}
	obj.SetDead(false)
	w.setDoorHP(obj, float64(obj.MaxHP())*restoreHP, true)
	obj.BroadcastRevive()
	return true
}

// RegenDoor implements task.DoorRegenEffects: it gives id's damaged door
// its regeneration for the tick due at due and broadcasts the new HP. A
// tick that a newer schedule replaced, or that reaches a broken or full
// door, does nothing.
func (w *WorldObjects) RegenDoor(id int, due time.Time) {
	obj, ok := w.Door(id)
	if !ok {
		return
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	if w.doorRegen.Tracked(id) || obj.Dead() || obj.CurrentHP() >= float64(obj.MaxHP()) {
		return
	}
	w.doorRegen.Next(id, due)
	w.setDoorHP(obj, obj.CurrentHP()+math.Max(1, door.HPRegen), false)
	obj.BroadcastStatus()
}

// setDoorHP sets obj's HP, capped at its maximum, starting its regeneration
// while below that maximum and stopping it once there; the caller holds
// doorMu. A broken door keeps its HP. broadcast sends the new HP to
// observers.
func (w *WorldObjects) setDoorHP(obj *door.Object, hp float64, broadcast bool) {
	if obj.Dead() {
		return
	}
	if maxHP := float64(obj.MaxHP()); hp >= maxHP {
		obj.StoreHP(maxHP)
		w.doorRegen.Cancel(obj.DoorID())
	} else {
		obj.StoreHP(hp)
		w.doorRegen.Begin(obj.DoorID())
	}
	if broadcast {
		obj.BroadcastStatus()
	}
}

// breakDoor kills obj: its HP drops to zero, it stops regenerating, and a
// closed door stops blocking geodata while keeping its closed state;
// observers see the zero HP twice, once as it is set and once for the
// death. The caller holds doorMu.
func (w *WorldObjects) breakDoor(obj *door.Object) {
	if obj.Dead() {
		return
	}
	w.setDoorHP(obj, 0, true)
	obj.SetDead(true)
	w.doorRegen.Cancel(obj.DoorID())
	obj.BroadcastStatus()
	if !obj.Opened() {
		w.geo.RemoveObject(obj)
	}
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
	if w.newSink != nil {
		obj.Attach(w.newSink(obj))
	}
	obj.SetOwner(w)
	obj.SetSight(doorSight{geo: w.geo})
	w.state.Spawn(obj, tmpl.Position.X, tmpl.Position.Y, tmpl.Position.Z, 0)
	if !obj.Opened() {
		w.geo.AddObject(obj)
	}
	w.scheduleDoorTimer(obj)
	return obj, nil
}

// doorSight runs a door's line-of-sight queries on the world geodata.
type doorSight struct{ geo *engine.Engine }

func (s doorSight) CanSeeActorIgnoring(ox, oy, oz int, oh float64, tx, ty, tz int, th float64, ignore door.GeoShape) bool {
	// A nil ignore converts to a nil dynamic.Object: nothing is left out.
	return s.geo.CanSeeActorIgnoring(ox, oy, oz, oh, tx, ty, tz, th, ignore)
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
