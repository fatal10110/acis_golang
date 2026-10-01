package manager

import (
	"fmt"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// FolkSpawner places civilian NPCs in the world, each on a queue of its
// own that its regeneration, effects, AI and walks run on, and on the AI
// task. A civilian whose template alias names a walker route
// (walkerRoutes.xml, keyed by that alias for both the route and the NPC)
// is given movement and walks the route from the moment it spawns. Every
// other one stands at its spawn point; one whose template can move is given
// movement too, to walk toward the target of a cast desire.
type FolkSpawner struct {
	State  *world.State
	Walker *task.Walker
	Geo    move.Geo
	// Positions ticks a moving NPC's position while it walks.
	Positions move.PositionUpdateRegistry
	Queues    Queues
	// NewSink builds the sink the NPC shows its movement and status
	// through; nil leaves them unseen.
	NewSink func(*npc.Folk) event.Sink
	Zones   *zone.Index
	// Skills resolves template passives into the NPC's stats and the skills
	// it casts; nil leaves it without a cast runtime.
	Skills actorcast.Definitions
	// CastEffects dispatches the effects of the NPC's casts.
	CastEffects actorcast.EffectHandlers
	// AI ticks the NPC once a second while its region is active; nil
	// leaves it without an AI.
	AI *task.AI
	// Items resolves the template's held weapon and shield.
	Items *item.Table
	// Decay removes a dead NPC's corpse; nil leaves corpses in place.
	Decay *task.Decay
	// Effects is the server's effect-list context.
	Effects effect.Env
	// MaxBuffsAmount is the configured base buff-slot count.
	MaxBuffsAmount      int
	MaxGeoPathFailCount int
	Log                 zerolog.Logger
}

// Spawn builds a civilian NPC from inst, places it at (loc, heading) and
// on the AI task, and starts its route walk when it has one.
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
		Items:          s.Items,
		Decay:          s.Decay,
	}
	if s.NewSink != nil {
		rt.Sink = s.NewSink(f)
	}
	if s.AI != nil {
		rt.AI = s.AI
	}
	if los, ok := s.Geo.(npc.LineOfSight); ok {
		rt.LOS = los
	}
	if err := f.Attach(rt); err != nil {
		return nil, err
	}
	if s.Skills != nil {
		ctl := actorcast.NewController(actorcast.FolkActor{Folk: f}, f.CastEvents())
		ctl.SetQueue(rt.Queue)
		f.SetCaster(ctl, &actorcast.AIController{
			Controller:  ctl,
			Definitions: s.Skills,
			Effects:     s.CastEffects,
			Caster:      f,
			// Target-addressed messages, such as a restored resource, reach
			// a player target whoever casts.
			OnHitResult: s.CastEffects.OnHitResult,
		})
	}
	alias := inst.Template.Alias
	walksRoute := s.Walker != nil && alias != "" && s.Walker.HasRoute(alias, alias)
	var control *folkControl
	if walksRoute || (inst.Template.CanMove && s.Geo != nil) {
		m := s.movement(rt)
		if walksRoute {
			control = &folkControl{walker: s.Walker, log: s.Log}
			m.Control, m.Route = control, s.Walker
		}
		walker, err := f.EnableMovement(m)
		if err != nil {
			return nil, err
		}
		if control != nil {
			control.ref = walker
		}
	}
	s.State.Spawn(f, loc.X, loc.Y, loc.Z, heading)
	// The AI and walker tasks only work actors already in the world grid.
	if s.AI != nil {
		s.AI.Add(f)
	}
	if control != nil {
		if err := s.Walker.StartRoute(control.ref, alias, alias); err != nil {
			s.Log.Warn().Err(err).Str("alias", alias).Msg("npc: folk route walk")
		}
	}
	return f, nil
}

// movement is what the NPC rt runs moves with.
func (s FolkSpawner) movement(rt npc.FolkRuntime) npc.FolkMovement {
	m := npc.FolkMovement{
		Geo:                 s.Geo,
		Queue:               rt.Queue,
		Positions:           s.Positions,
		World:               s.State,
		Sink:                rt.Sink,
		MaxGeoPathFailCount: s.MaxGeoPathFailCount,
		Log:                 s.Log,
	}
	if s.Zones != nil {
		m.WaterSurface = waterSurface(s.Zones)
		m.InWater = func(at location.Location) bool {
			_, ok := zone.FindAt[*zone.Water](s.Zones, at.X, at.Y, at.Z)
			return ok
		}
	}
	return m
}

// folkControl hands a walking civilian NPC's arrivals to the route walker
// task. Spawn fills ref before the first walk starts.
type folkControl struct {
	walker *task.Walker
	ref    *npc.FolkWalker
	log    zerolog.Logger
}

// Emit advances the NPC's route on each arrival, and ends it when the NPC
// dies.
func (c *folkControl) Emit(ev event.Event) {
	switch ev.(type) {
	case event.Arrived:
		if err := c.walker.Arrived(c.ref); err != nil {
			c.log.Warn().Err(err).Msg("task: walker arrived")
		}
	case event.Died:
		c.walker.StopRoute(c.ref)
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
