package manager

import (
	"fmt"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// FolkSpawner places civilian NPCs in the world, each on a queue of its
// own that its regeneration, effects and walk run on. A civilian whose
// template alias names a walker route (walkerRoutes.xml, keyed by that
// alias for both the route and the NPC) is given movement and walks the
// route from the moment it spawns; every other one stands at its spawn
// point.
type FolkSpawner struct {
	State  *world.State
	Walker *task.Walker
	Geo    move.Geo
	// Positions ticks a walker's position while it walks.
	Positions move.PositionUpdateRegistry
	Queues    Queues
	// NewSink builds the sink the NPC shows its movement and status
	// through; nil leaves them unseen.
	NewSink func(*npc.Folk) event.Sink
	Zones   *zone.Index
	// Skills resolves template passives into the NPC's stats.
	Skills actorcast.Definitions
	// Effects is the server's effect-list context.
	Effects effect.Env
	// MaxBuffsAmount is the configured base buff-slot count.
	MaxBuffsAmount      int
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
	rt := npc.FolkRuntime{
		World:          s.State,
		Queue:          s.Queues.NewQueue(fmt.Sprintf("npc-%d", inst.ObjectID)),
		Effects:        s.Effects,
		MaxBuffsAmount: s.MaxBuffsAmount,
	}
	if s.NewSink != nil {
		rt.Sink = s.NewSink(f)
	}
	if err := f.Attach(rt); err != nil {
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
		Queue:               rt.Queue,
		Positions:           s.Positions,
		World:               s.State,
		Sink:                rt.Sink,
		MaxGeoPathFailCount: s.MaxGeoPathFailCount,
		Control:             control,
		Log:                 s.Log,
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

// waterSurface reports the water zone at a position and its surface level,
// the mover's move.CreatureMove.SetWaterSurface query.
func waterSurface(zones *zone.Index) func(location.Location) (int, bool) {
	return func(position location.Location) (int, bool) {
		water, ok := zone.FindAt[*zone.Water](zones, position.X, position.Y, position.Z)
		if !ok {
			return 0, false
		}
		return water.WaterLevel(), true
	}
}
