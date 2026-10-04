package gameservertest

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// WithDoors spawns tmpls at boot through the production world-object owner,
// with the production door sinks, so a suite can target a door and observe
// its state changes on the wire. Each door stands on flat geodata at its
// template Z; the door timer task is never started, so no auto open/close
// fires during a test.
func WithDoors(tmpls ...*door.Template) Option {
	return func(o *options) { o.doors = append(o.doors, tmpls...) }
}

type idleDoorTimers struct{}

func (idleDoorTimers) ToggleDoor(int) {}

func bootDoors(t *testing.T, tmpls []*door.Template, ids *sequentialIDs, state *world.State) (*gamemanager.WorldObjects, *doorHarness) {
	t.Helper()
	geo := engine.New()
	for _, tmpl := range tmpls {
		region := block.NewRegion()
		for i := range block.RegionBlockCount {
			region.SetFlat(i, int16(tmpl.Position.Z))
		}
		regionX := engine.TileXMin + (tmpl.Position.X-engine.WorldXMin)/engine.TileSize
		regionY := engine.TileYMin + (tmpl.Position.Y-engine.WorldYMin)/engine.TileSize
		if err := geo.SetRegion(regionX, regionY, region); err != nil {
			t.Fatalf("door geodata: %v", err)
		}
	}
	doors, err := door.NewTable(tmpls)
	if err != nil {
		t.Fatalf("door table: %v", err)
	}
	statics, err := staticobject.NewTable(nil)
	if err != nil {
		t.Fatalf("static object table: %v", err)
	}
	timers, err := task.NewDoor(idleDoorTimers{}, time.Now)
	if err != nil {
		t.Fatalf("door timers: %v", err)
	}
	h := &doorHarness{geo: geo, clock: &autosaveClock{now: time.Now()}}
	h.regen = task.NewDoorRegen(h.clock.Now)
	objs, err := gamemanager.NewWorldObjects(doors, statics, ids, geo, state, timers, h.regen, network.DoorSinks(state), zerolog.Nop())
	if err != nil {
		t.Fatalf("world objects: %v", err)
	}
	return objs, h
}

// doorHarness keeps the geodata WithDoors' doors block, and their production
// regeneration schedule on a clock (the mutex-guarded harness clock autosave
// uses too) that only AdvanceDoorRegen moves and sweeps, instead of a
// ticker.
type doorHarness struct {
	geo   *engine.Engine
	clock *autosaveClock
	regen *task.DoorRegen
}

// DoorGeo returns the geodata engine WithDoors' doors stand on.
func (s *Server) DoorGeo(tb testing.TB) *engine.Engine {
	tb.Helper()
	if s.doors == nil {
		tb.Fatal("DoorGeo needs WithDoors")
	}
	return s.doors.geo
}

// AdvanceDoorRegen lets d pass on the door regeneration clock and runs the
// regeneration ticks that fall due, through the world objects that own the
// doors.
func (s *Server) AdvanceDoorRegen(tb testing.TB, d time.Duration) {
	tb.Helper()
	if s.doors == nil {
		tb.Fatal("AdvanceDoorRegen needs WithDoors")
	}
	s.doors.clock.Advance(d)
	s.doors.regen.Tick(s.WorldObjects)
}

// DoorRegenerating reports whether doorID's door has a regeneration tick
// pending.
func (s *Server) DoorRegenerating(tb testing.TB, doorID int) bool {
	tb.Helper()
	if s.doors == nil {
		tb.Fatal("DoorRegenerating needs WithDoors")
	}
	return s.doors.regen.Tracked(doorID)
}

// siegeInProgress stands in for the siege of a door's residence while the
// castle sieges install no door gate (#3373): it lets every hit through, as
// a castle siege in progress does for any attacker but a Swoop Cannon.
type siegeInProgress struct{}

func (siegeInProgress) AllowsDoorDamage(attackable.Combatant) bool { return true }

// BeginDoorSiege puts doorID's door under a siege that lets hits lower its
// HP.
func (s *Server) BeginDoorSiege(tb testing.TB, doorID int) {
	tb.Helper()
	s.spawnedDoor(tb, doorID).SetSiege(siegeInProgress{})
}

// HitDoor lands damage on doorID's door from the online player
// attackerObjID, through the door's own damage entry point.
func (s *Server) HitDoor(tb testing.TB, doorID int, attackerObjID int32, damage float64) {
	tb.Helper()
	s.spawnedDoor(tb, doorID).ReduceHP(damage, s.onlineCharacter(tb, attackerObjID))
}

func (s *Server) spawnedDoor(tb testing.TB, doorID int) *door.Object {
	tb.Helper()
	obj, ok := s.WorldObjects.Door(doorID)
	if !ok {
		tb.Fatalf("door %d is not spawned", doorID)
	}
	return obj
}
