package gameservertest

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
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

func bootDoors(t *testing.T, tmpls []*door.Template, ids *sequentialIDs, state *world.State) *gamemanager.WorldObjects {
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
	objs, err := gamemanager.NewWorldObjects(doors, statics, ids, geo, state, timers, network.DoorSinks(state), zerolog.Nop())
	if err != nil {
		t.Fatalf("world objects: %v", err)
	}
	return objs
}
