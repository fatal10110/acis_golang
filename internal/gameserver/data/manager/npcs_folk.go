package manager

import (
	"fmt"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// FolkSpawner places civilian NPCs in the world. A civilian whose template
// alias names a walker route (walkerRoutes.xml, keyed by that alias for
// both the route and the NPC) is given movement and walks the route from
// the moment it spawns; every other one stands at its spawn point.
type FolkSpawner struct {
	State  *world.State
	Walker *task.Walker
	Geo    move.Geo
	// Positions ticks a walker's position while it walks.
	Positions move.PositionUpdateRegistry
	Queues    Queues
	// NewSink builds the sink a walker shows its movement through; nil
	// leaves it unseen.
	NewSink func(*npc.Folk) event.Sink
	Zones   *zone.Index
	// Skills resolves template passives into the NPC's fixed stats.
	Skills              actorcast.Definitions
	MaxGeoPathFailCount int
	Log                 zerolog.Logger
}

// Spawn builds a civilian NPC from inst and places it at (loc, heading),
// starting its route walk when it has one.
func (s FolkSpawner) Spawn(inst *npc.Instance, loc location.Location, heading int) (*npc.Folk, error) {
	inPeace := s.Zones != nil && s.Zones.NPCInPeaceZone(loc.X, loc.Y, loc.Z)
	f, err := npc.NewFolk(inst, inPeace, s.Skills)
	if err != nil {
		return nil, err
	}
	alias := inst.Template.Alias
	if s.Walker == nil || alias == "" || !s.Walker.HasRoute(alias, alias) {
		s.State.Spawn(f, loc.X, loc.Y, loc.Z, heading)
		return f, nil
	}

	control := &folkControl{walker: s.Walker, log: s.Log}
	m := npc.FolkMovement{
		Geo:                 s.Geo,
		Queue:               s.Queues.NewQueue(fmt.Sprintf("npc-%d", inst.ObjectID)),
		Positions:           s.Positions,
		World:               s.State,
		MaxGeoPathFailCount: s.MaxGeoPathFailCount,
		Control:             control,
		Log:                 s.Log,
	}
	if s.NewSink != nil {
		m.Sink = s.NewSink(f)
	}
	if s.Zones != nil {
		m.WaterSurface = waterSurface(s.Zones)
		m.InWater = func(at location.Location) bool {
			_, ok := zone.FindAt[*zone.Water](s.Zones, at.X, at.Y, at.Z)
			return ok
		}
	}
	walker, err := f.EnableMovement(m)
	if err != nil {
		return nil, err
	}
	control.ref = walker
	s.State.Spawn(f, loc.X, loc.Y, loc.Z, heading)
	// The walker only works actors already in the world grid.
	if err := s.Walker.StartRoute(walker, alias, alias); err != nil {
		s.Log.Warn().Err(err).Str("alias", alias).Msg("npc: folk route walk")
	}
	return f, nil
}

// folkControl hands a walking civilian NPC's arrivals to the route walker
// task. Spawn fills ref before the first walk starts.
type folkControl struct {
	walker *task.Walker
	ref    *npc.FolkWalker
	log    zerolog.Logger
}

// Emit advances the NPC's route on each arrival.
func (c *folkControl) Emit(ev event.Event) {
	if _, ok := ev.(event.Arrived); !ok {
		return
	}
	if err := c.walker.Arrived(c.ref); err != nil {
		c.log.Warn().Err(err).Msg("task: walker arrived")
	}
}

// waterSurface caps a walk under water at the surface of the water zone it
// is in.
func waterSurface(zones *zone.Index) func(location.Location, int) (int, bool) {
	return func(position location.Location, groundZ int) (int, bool) {
		water, ok := zone.FindAt[*zone.Water](zones, position.X, position.Y, position.Z)
		if !ok || groundZ-water.WaterLevel() >= -20 {
			return 0, false
		}
		return water.WaterLevel(), true
	}
}
