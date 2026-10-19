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
	// sieges are the residence sieges SetDoorSiege installed, by door id,
	// installed again on every door a reload builds under that id.
	// Guarded by doorMu.
	sieges map[int]door.Siege
	// hpUpgrades are the HP upgrade ratios UpgradeDoor set, by door id,
	// set again on every door a reload builds under that id. Guarded by
	// doorMu.
	hpUpgrades map[int]int
	// watchers are the watchers WatchDoor registered, by standing door.
	// Guarded by doorMu.
	watchers map[*door.Object]*doorWatchers
	// clanCall is the call SetClanCall installed, installed again on every
	// door a reload builds. Guarded by doorMu.
	clanCall door.ClanCall

	// doorMu serializes door state changes, so a flag flip or an HP change
	// and its geodata, broadcast and timer follow-up land as one step
	// whichever goroutine (the door timer, the regeneration sweep, a
	// player's unlock or hit, a reload) makes it. A change looks its door
	// up under doorMu, so it never lands on a door a reload replaced. A
	// status broadcast under doorMu reads the door's residence siege,
	// which takes that siege's own lock; a door's death is told to the
	// siege once doorMu is released.
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

// DoorByName returns the spawned door named name, ignoring case.
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

// SetClanCall installs c as the call a hit on any door makes to the NPCs
// around it, on the doors standing and on every door a reload builds later.
func (w *WorldObjects) SetClanCall(c door.ClanCall) {
	if w == nil {
		return
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	w.clanCall = c
	for _, obj := range w.doors.Load().order {
		obj.SetClanCall(c)
	}
}

// SetDoorSiege makes s the siege of the residence door id belongs to, on
// the door standing and on every door a reload builds under that id; a
// nil s removes it.
func (w *WorldObjects) SetDoorSiege(id int, s door.Siege) {
	if w == nil {
		return
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	if s == nil {
		delete(w.sieges, id)
	} else {
		if w.sieges == nil {
			w.sieges = make(map[int]door.Siege)
		}
		w.sieges[id] = s
	}
	if obj, ok := w.Door(id); ok {
		obj.SetSiege(s)
	}
}

// UpgradeDoor multiplies id's door's template HP by ratio into its maximum
// HP, on the door standing and on every door a reload builds under that
// id, then fills the standing door's HP to that maximum, observers seeing
// it. A broken door keeps its HP. It reports whether the door stands in
// the world.
func (w *WorldObjects) UpgradeDoor(id, ratio int) bool {
	if w == nil {
		return false
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok {
		return false
	}
	w.setHPUpgrade(id, ratio)
	obj.StoreHPUpgradeRatio(ratio)
	w.setDoorHP(obj, float64(obj.MaxHP()), true)
	return true
}

// ResetDoorUpgrade takes id's door's HP upgrade back to none, leaving its
// HP as it is: the siege change of hands that resets it stands the door
// again at once.
func (w *WorldObjects) ResetDoorUpgrade(id int) {
	if w == nil {
		return
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	w.setHPUpgrade(id, 1)
	if obj, ok := w.Door(id); ok {
		obj.StoreHPUpgradeRatio(1)
	}
}

// setHPUpgrade records id's HP upgrade ratio for the doors a reload
// builds; the caller holds doorMu.
func (w *WorldObjects) setHPUpgrade(id, ratio int) {
	if ratio == 1 {
		delete(w.hpUpgrades, id)
		return
	}
	if w.hpUpgrades == nil {
		w.hpUpgrades = make(map[int]int)
	}
	w.hpUpgrades[id] = ratio
}

// SetDoorOpen changes a door's open state, applies the matching geodata,
// broadcasts the change to known observers, runs the door's watchers
// (WatchDoor), propagates the same state to a linked controller door
// (Template.TriggeredID), and reschedules the door's next auto open/close
// timer from its template's openTime/closeTime/randomTime.
func (w *WorldObjects) SetDoorOpen(id int, open bool) bool {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok {
		return false
	}
	return w.changeDoorState(obj, open, false, inlineDoorRun())
}

// changeDoorState opens or closes obj as open says; the caller
// holds doorMu. A broken door keeps its state. triggered is true only for a
// cascaded change propagated from another door's Template.TriggeredID, and
// suppresses this door's own auto-timer reschedule so the linked door's
// cascade doesn't double-schedule it. The door's watchers run as run says,
// after its broadcast and before the linked door changes; run inline
// releases doorMu around them, so another change may land meanwhile, and a
// door a reload replaced meanwhile gets no timer.
func (w *WorldObjects) changeDoorState(obj *door.Object, open, triggered bool, run *doorRun) bool {
	if obj.Dead() || !obj.SetOpened(open) {
		return false
	}
	if open {
		w.geo.RemoveObject(obj)
	} else {
		w.geo.AddObject(obj)
	}
	obj.BroadcastStatus()
	w.watchDoorChange(obj, run)
	if obj.Template.TriggeredID > 0 {
		if linked, ok := w.Door(obj.Template.TriggeredID); ok {
			w.changeDoorState(linked, open, true, run)
		}
	}
	if !triggered {
		if current, ok := w.Door(obj.DoorID()); ok && current == obj {
			w.scheduleDoorTimer(obj)
		}
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
	w.changeDoorState(obj, !obj.Opened(), false, inlineDoorRun())
}

// ReduceDoorHP implements door.StateOwner: it takes amount off id's door HP
// once door.Object.ReduceHP let the hit through, broadcasting the new HP,
// and breaks the door once less than half a point is left, telling its
// residence once the death is applied. A broken door takes no more damage.
func (w *WorldObjects) ReduceDoorHP(id int, amount float64) {
	if broken := w.reduceDoorHP(id, amount); broken != nil {
		broken.TellBroken()
	}
}

// reduceDoorHP is ReduceDoorHP's state change, under doorMu; it returns
// the door it broke, nil for none.
func (w *WorldObjects) reduceDoorHP(id int, amount float64) *door.Object {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok || obj.Dead() {
		return nil
	}
	if amount > 0 {
		w.setDoorHP(obj, math.Max(obj.CurrentHP()-amount, 0), true)
	}
	if obj.CurrentHP() >= 0.5 {
		return nil
	}
	w.breakDoor(obj)
	return obj
}

// TakeDoorHP implements door.StateOwner: it takes amount straight off
// id's standing door's HP, as a REAL_DAMAGE skill does, breaking the door
// when that leaves zero HP or less and telling its residence once the
// death is applied, else setting the HP left and broadcasting it.
func (w *WorldObjects) TakeDoorHP(id int, amount float64) {
	if broken := w.takeDoorHP(id, amount); broken != nil {
		broken.TellBroken()
	}
}

// takeDoorHP is TakeDoorHP's state change, under doorMu; it returns the
// door it broke, nil for none.
func (w *WorldObjects) takeDoorHP(id int, amount float64) *door.Object {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok || obj.Dead() {
		return nil
	}
	left := obj.CurrentHP() - amount
	if left > 0 {
		w.setDoorHP(obj, left, true)
		return nil
	}
	w.breakDoor(obj)
	return obj
}

// ReviveDoor stands id's broken door again, as a siege's door respawn does:
// it takes its template's open state back, blocking geodata again when that
// state is closed, regains restoreHP of its maximum HP, and observers see
// the HP and then the revive. It reports whether the door was broken.
func (w *WorldObjects) ReviveDoor(id int, restoreHP float64) bool {
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	return ok && w.reviveDoor(obj, restoreHP)
}

// RespawnDoor stands id's door again as a siege's door respawn does: a
// broken door revives as ReviveDoor does, regaining restoreHP of its
// maximum HP; the door closes; then its HP is set to its maximum, or to
// half of it, rounded down, when weak. Observers see each change as it
// lands, the HP always.
//
// The door's watchers run once the respawn is done.
func (w *WorldObjects) RespawnDoor(id int, restoreHP float64, weak bool) {
	run := &doorRun{}
	defer run.fire()
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok {
		return
	}
	w.reviveDoor(obj, restoreHP)
	w.changeDoorState(obj, false, false, run)
	hp := obj.MaxHP()
	if weak {
		hp /= 2
	}
	w.setDoorHP(obj, float64(hp), true)
}

// reviveDoor is ReviveDoor on obj; the caller holds doorMu.
func (w *WorldObjects) reviveDoor(obj *door.Object, restoreHP float64) bool {
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

// ReloadDoors replaces every spawned door with one built from doors. Each door
// standing opens first, as an open does, so no blocker of an old door is in the
// geodata the new doors are measured against. The new doors are built, the old
// ones leave the world with their timers and regeneration, and each new door
// enters the world in the state opposite to its template's, so observers see it
// so first, and then takes its template's state as any state change does
// (geodata, broadcast, a linked new door, the auto timer); a linked door not in
// the world yet takes the change too. When a new door cannot be built, the
// doors that opened close again and stay.
//
// Doors are looked up by id when they act, so scripts, clan hall grounds,
// keys, timers and regeneration reach the new doors, and so do the
// residence siege SetDoorSiege installed and the HP upgrade UpgradeDoor set
// under a door's id, the new door standing at its upgraded maximum HP, and
// the clan call SetClanCall installed. A door change
// that comes in during the reload waits for it and lands on the new door.
//
// The watchers of an old door (WatchDoor) see it open, once the reload is
// done; the new doors have none.
func (w *WorldObjects) ReloadDoors(doors *door.Table) error {
	run := &doorRun{}
	defer run.fire()
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	old := w.doors.Load().order
	var opened []*door.Object
	for _, obj := range old {
		if w.changeDoorState(obj, true, false, run) {
			opened = append(opened, obj)
		}
	}
	fresh, err := w.newDoors(doors)
	if err != nil {
		for _, obj := range opened {
			w.changeDoorState(obj, false, false, run)
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
		delete(w.watchers, obj)
	}
	w.doors.Store(fresh)
	for _, obj := range fresh.order {
		pos := obj.Template.Position
		w.state.Spawn(obj, pos.X, pos.Y, pos.Z, 0)
		w.changeDoorState(obj, obj.Template.Opened, false, run)
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
	obj.SetClanCall(w.clanCall)
	if s, ok := w.sieges[tmpl.ID]; ok {
		obj.SetSiege(s)
	}
	if ratio, ok := w.hpUpgrades[tmpl.ID]; ok {
		obj.StoreHPUpgradeRatio(ratio)
		obj.StoreHP(float64(obj.MaxHP()))
	}
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
