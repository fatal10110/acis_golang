package manager

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
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
	ids        idAllocator
	log        zerolog.Logger
	now        func() time.Time
	// strife is the Seven Signs state door defences read, installed on
	// every door built, at boot and by ReloadDoors. Guarded by doorMu.
	strife door.Strife

	// doorMu serializes door state changes, so a flag flip or an HP change
	// and its geodata, broadcast and timer follow-up land as one step
	// whichever goroutine (the door timer, the regeneration sweep, a
	// player's unlock or hit, a reload) makes it. A change looks its door
	// up under doorMu, so it never lands on a door a reload replaced.
	doorMu sync.Mutex

	// doors is replaced whole, under doorMu, by ReloadDoors; lookups read
	// it without the lock.
	doors       atomic.Pointer[doorSet]
	staticOrder []*staticobject.Object
}

// doorSet is the spawned doors, by door id and in template order. It is
// never changed once published.
type doorSet struct {
	byID  map[int]*door.Object
	order []*door.Object
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
		ids:        ids,
		log:        log,
		now:        time.Now,
	}
	set, err := w.newDoors(doors)
	if err != nil {
		return nil, err
	}
	w.doors.Store(set)
	for _, obj := range set.order {
		w.spawnDoor(obj)
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
	obj, ok := w.doors.Load().byID[id]
	return obj, ok
}

// DoorByName returns the spawned door named name, ignoring case
// (DoorData.getDoor(String)).
func (w *WorldObjects) DoorByName(name string) (*door.Object, bool) {
	if w == nil {
		return nil, false
	}
	for _, obj := range w.doors.Load().order {
		if strings.EqualFold(obj.Template.Name, name) {
			return obj, true
		}
	}
	return nil, false
}

// Doors returns spawned doors in template order.
func (w *WorldObjects) Doors() []*door.Object {
	if w == nil {
		return nil
	}
	return append([]*door.Object(nil), w.doors.Load().order...)
}

// StaticObjects returns spawned static objects in template order.
func (w *WorldObjects) StaticObjects() []*staticobject.Object {
	if w == nil {
		return nil
	}
	return append([]*staticobject.Object(nil), w.staticOrder...)
}

// SetStrife installs the Seven Signs state every door's defences read, on
// the doors standing and on every door a reload builds later.
func (w *WorldObjects) SetStrife(s door.Strife) {
	if w == nil {
		return
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	w.strife = s
	for _, obj := range w.doors.Load().order {
		obj.SetStrife(s)
	}
}

// SetDoorOpen changes a door's open state, applies the matching geodata,
// broadcasts the change to known observers, reschedules the door's next auto
// open/close timer from its template's openTime/closeTime/randomTime, and
// propagates the same state to a linked controller door (Template.TriggeredID).
func (w *WorldObjects) SetDoorOpen(id int, open bool) bool {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok {
		return false
	}
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
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok {
		return
	}
	w.changeDoorState(obj, !obj.Opened(), false)
}

// ReduceDoorHP implements door.StateOwner: it takes amount off id's door HP
// once door.Object.ReduceHP let the hit through, broadcasting the new HP,
// and breaks the door once less than half a point is left. A broken door
// takes no more damage.
func (w *WorldObjects) ReduceDoorHP(id int, amount float64) {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok || obj.Dead() {
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
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok || !obj.Dead() {
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
// its regeneration for the tick due at due and broadcasts the new HP. A hit
// that landed while the tick was in flight leaves it due, so it still
// applies; a tick that a newer schedule replaced does nothing, and one that
// reaches a broken or full door stops the door's regeneration.
func (w *WorldObjects) RegenDoor(id int, due time.Time) {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok {
		w.doorRegen.Cancel(id)
		return
	}
	if !w.doorRegen.Due(id, due) {
		return
	}
	if obj.Dead() || obj.CurrentHP() >= float64(obj.MaxHP()) {
		w.doorRegen.Cancel(id)
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

// newDoors builds a door for every template of doors, not yet spawned. A
// door whose triangulated footprint is degenerate or samples to no geodata
// cells is logged and left out.
func (w *WorldObjects) newDoors(doors *door.Table) (*doorSet, error) {
	set := &doorSet{byID: make(map[int]*door.Object)}
	for _, tmpl := range doors.All() {
		obj, err := w.newDoor(tmpl)
		if err != nil {
			if errors.Is(err, door.ErrEmptyFootprint) || errors.Is(err, dynamic.ErrDegenerateFootprint) {
				w.log.Warn().Err(err).Int("door", tmpl.ID).Msg("data/manager: skipping door with degenerate footprint")
				continue
			}
			return nil, err
		}
		set.byID[obj.DoorID()] = obj
		set.order = append(set.order, obj)
	}
	return set, nil
}

// ReloadDoors replaces every spawned door with one built from doors
// (DoorData.reload). Each door standing opens first, as an open does, so
// no blocker of an old door is in the geodata the new doors are measured
// against. The new doors are built, the old ones leave the world with
// their timers and regeneration, and each new door enters the world in
// the state opposite to its template's, so observers see it so first, and
// then takes its template's state as any state change does (geodata,
// broadcast, a linked new door, the auto timer); a linked door not in the
// world yet takes the change too. When a new door cannot be built, the
// doors that opened close again and stay.
//
// Doors are looked up by id when they act, so scripts, clan hall grounds,
// keys, timers and regeneration reach the new doors. No siege gate is
// carried over: a new door has none, as one built at boot. A door change
// that comes in during the reload waits for it and lands on the new door.
func (w *WorldObjects) ReloadDoors(doors *door.Table) error {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	old := w.doors.Load().order
	var opened []*door.Object
	for _, obj := range old {
		if w.changeDoorState(obj, true, false) {
			opened = append(opened, obj)
		}
	}
	fresh, err := w.newDoors(doors)
	if err != nil {
		for _, obj := range opened {
			w.changeDoorState(obj, false, false)
		}
		return err
	}
	for _, obj := range fresh.order {
		obj.SetOpened(!obj.Template.Opened)
	}
	for _, obj := range old {
		w.doorTimers.Cancel(obj.DoorID())
		w.doorRegen.Cancel(obj.DoorID())
		w.state.Despawn(obj)
	}
	w.doors.Store(fresh)
	for _, obj := range fresh.order {
		pos := obj.Template.Position
		w.state.Spawn(obj, pos.X, pos.Y, pos.Z, 0)
		w.changeDoorState(obj, obj.Template.Opened, false)
	}
	return nil
}

// newDoor builds the door of tmpl, not yet spawned.
func (w *WorldObjects) newDoor(tmpl *door.Template) (*door.Object, error) {
	id, err := w.ids.NextID()
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
	obj.SetStrife(w.strife)
	return obj, nil
}

// spawnDoor puts obj into the world as the boot does: in its template's
// state, its geodata blocker in place when closed, its auto timer
// scheduled.
func (w *WorldObjects) spawnDoor(obj *door.Object) {
	pos := obj.Template.Position
	w.state.Spawn(obj, pos.X, pos.Y, pos.Z, 0)
	if !obj.Opened() {
		w.geo.AddObject(obj)
	}
	w.scheduleDoorTimer(obj)
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
