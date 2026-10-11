package npc

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// FolkMovement is what a civilian NPC that can move moves with: along its
// route, if it has one, and toward the target of a cast desire. A civilian
// NPC given none stands where it spawned.
type FolkMovement struct {
	Geo   move.Geo
	Queue *sim.Queue
	// Positions ticks the NPC's position while a walk is under way.
	Positions move.PositionUpdateRegistry
	World     *world.State
	// Sink receives the NPC's moves, stops, teleports and route lines for
	// its observers; nil leaves them unseen.
	Sink event.Sink
	// WaterSurface reports the water zone at a position and its surface
	// level, where the NPC swims; nil for none.
	WaterSurface func(location.Location) (int, bool)
	// InWater reports whether a teleport destination lies in water, where
	// it keeps its own height; nil for never.
	InWater func(location.Location) bool
	// MaxGeoPathFailCount is the failed-pathfinding streak past which the
	// streak restarts from zero; zero keeps DefaultMaxGeoPathFailCount.
	MaxGeoPathFailCount int
	// Control receives event.Arrived, on Queue, each time a walk reaches
	// its destination, after the NPC's position settles there, and
	// event.Died when the NPC dies; nil for none.
	Control event.Sink
	// Route is the route walk task the NPC's route desires walk it on; nil
	// leaves those desires moving it nowhere.
	Route FolkRoute
	// Log reports a failed-pathfinding streak that overflows.
	Log zerolog.Logger
}

// FolkRoute is the route walk task a walking civilian NPC walks its route
// desires on (see task.Walker): Walk puts it on a route, LeaveRoute takes it
// off while its AI acts on another desire.
type FolkRoute interface {
	Walk(actor task.WalkerActor, routeName, npcName string) error
	LeaveRoute(task.WalkerActor)
}

// folkMotion is a movable civilian NPC's movement: the moving actor its
// controller drives and the sink of that controller's events.
type folkMotion struct {
	*Folk
	cfg    FolkMovement
	move   *move.CreatureMove
	ctl    *move.Controller
	walker *FolkWalker

	geoPathFails atomic.Int32
	teleporting  atomic.Bool
}

// FolkWalker is the route walking surface of a civilian NPC given movement.
type FolkWalker struct{ *folkMotion }

// EnableMovement gives f the movement m describes, at f's spawn point and
// move speed, and returns its route walking surface. Call it once, before f
// is published into the world.
func (f *Folk) EnableMovement(m FolkMovement) (*FolkWalker, error) {
	if f.motion != nil {
		return nil, errors.New("npc: folk movement already enabled")
	}
	if m.Queue == nil {
		return nil, errors.New("npc: folk movement needs a queue")
	}
	cm, err := move.NewCreatureMove(f.Instance.Home, f.MoveSpeed(), m.Geo)
	if err != nil {
		return nil, fmt.Errorf("npc %d folk movement: %w", f.Instance.Template.ID, err)
	}
	cm.SetQueue(m.Queue)
	if m.WaterSurface != nil {
		cm.SetWaterSurface(m.WaterSurface)
	}
	if f.zones.ix != nil {
		// The NPC swims while its water zones hold it, not wherever the
		// water query finds it.
		cm.UseCreatureZoneSwim()
	}
	motion := &folkMotion{Folk: f, cfg: m, move: cm}
	ctl, err := move.NewController(cm, motion, motion)
	if err != nil {
		return nil, err
	}
	if m.Positions != nil {
		ctl.SetPositionUpdates(m.Positions)
	}
	motion.ctl = ctl
	motion.walker = &FolkWalker{folkMotion: motion}
	f.motion = motion
	return motion.walker, nil
}

// IsMoving reports whether the NPC is walking.
func (f *Folk) IsMoving() bool {
	return f.motion != nil && f.motion.move.Moving()
}

// MovingTo returns the target of the NPC's leg in flight and whether it is
// walking at all.
func (f *Folk) MovingTo() (location.Location, bool) {
	if f.motion == nil {
		return location.Location{}, false
	}
	return f.motion.move.MovingTo()
}

// Emit settles the NPC's controller events: an arrival syncs its world
// position, faces its spawn heading when back on its spawn point, and is
// passed on to Control; a blocked walk shows observers where it stopped.
func (m *folkMotion) Emit(ev event.Event) {
	switch ev.(type) {
	case event.Arrived:
		at := m.ctl.Position()
		m.SyncPosition(at)
		// A move's end revalidates the zones at once.
		m.zones.settle()
		if at == m.Instance.Home {
			m.SetHeading(m.Instance.SpawnHeading)
		}
		if m.cfg.Control != nil {
			m.cfg.Control.Emit(ev)
		}
	case event.MoveBlocked:
		m.ctl.BroadcastBlockedCorrection()
	}
}

func (m *folkMotion) emit(ev event.Event) {
	if m.cfg.Sink != nil {
		m.cfg.Sink.Emit(ev)
	}
}

// SyncPosition moves the NPC's world presence to position, a movement step.
func (m *folkMotion) SyncPosition(position location.Location) {
	if m.cfg.World != nil {
		x, y, z := m.Folk.Position()
		_ = m.cfg.World.Move(m.Folk, position.X, position.Y, position.Z)
		m.zones.step(location.Location{X: x, Y: y, Z: z})
	}
}

// BroadcastMove shows observers the walk.
func (m *folkMotion) BroadcastMove(ev event.Move) { m.emit(ev) }

// BroadcastStop shows observers a stop in place, once the stop has
// revalidated the zones.
func (m *folkMotion) BroadcastStop() {
	m.zones.settle()
	m.emit(event.Stopped{})
}

// OwnsOffensiveFollowTicker reports false: the controller rechecks the
// offensive follow a cast desire starts.
func (m *folkMotion) OwnsOffensiveFollowTicker() bool { return false }

// OffensiveFollowLead reports true: as any NPC, the NPC reaches fifty
// further for a target on the move.
func (*folkMotion) OffensiveFollowLead() bool { return true }

// IntentionMovesToTarget reports whether the cast desire the NPC's AI acts
// on lets it walk to its target.
func (m *folkMotion) IntentionMovesToTarget() bool {
	m.cast.currentMu.Lock()
	defer m.cast.currentMu.Unlock()
	return m.cast.current != nil && m.cast.current.MoveToTarget
}

// CanSee reports whether the NPC has line of sight to target.
func (m *folkMotion) CanSee(target attackable.Combatant) bool { return m.canSee(target) }

// MovementDisabled reports a template that cannot move, a dead NPC or a
// teleport under way; nothing else immobilizes a civilian NPC.
func (m *folkMotion) MovementDisabled() bool {
	return !m.Instance.Template.CanMove || m.Dead() || m.teleporting.Load()
}

// GeoPathFailCount reports the NPC's failed-pathfinding streak.
func (m *folkMotion) GeoPathFailCount() int { return int(m.geoPathFails.Load()) }

// ResetGeoPathFailCount clears the failed-pathfinding streak.
func (m *folkMotion) ResetGeoPathFailCount() { m.geoPathFails.Store(0) }

// AddGeoPathFailCount counts one more failed pathfinding attempt; past the
// configured maximum the streak is logged and restarts from zero instead.
func (m *folkMotion) AddGeoPathFailCount() {
	limit := int32(m.cfg.MaxGeoPathFailCount)
	if limit <= 0 {
		limit = DefaultMaxGeoPathFailCount
	}
	for {
		cur := m.geoPathFails.Load()
		if cur <= limit {
			if m.geoPathFails.CompareAndSwap(cur, cur+1) {
				return
			}
			continue
		}
		if m.geoPathFails.CompareAndSwap(cur, 0) {
			x, y, z := m.Folk.Position()
			m.cfg.Log.Warn().
				Str("npc", m.Instance.Template.Name).
				Int("x", x).
				Int("y", y).
				Int("z", z).
				Int("heading", m.Heading()).
				Msg("geopath fail overflow")
			return
		}
	}
}

// TeleportTo jumps the NPC to target: any walk stops, target is grounded
// unless it lies in water, observers see the jump, the NPC leaves and
// re-enters the world grid there, and its failed-pathfinding streak clears.
// A teleport that starts while another is under way is dropped.
func (m *folkMotion) TeleportTo(target location.Location) {
	if !m.teleporting.CompareAndSwap(false, true) {
		return
	}
	m.ctl.Stop()
	if m.cfg.InWater == nil || !m.cfg.InWater(target) {
		target.Z = int(m.move.Height(target.X, target.Y, target.Z))
	}
	m.emit(event.Teleported{To: target})
	// The NPC leaves the zones around its old position while its observers
	// there still know it, and enters those at target once it has landed.
	x, y, z := m.Folk.Position()
	m.zones.leave(location.Location{X: x, Y: y, Z: z})
	m.move.SetPosition(target)
	if m.cfg.World != nil {
		_ = m.cfg.World.Teleport(m.Folk, target.X, target.Y, target.Z)
	}
	m.teleporting.Store(false)
	m.zones.enter()
	m.ResetGeoPathFailCount()
}

// Queue returns the queue the NPC's movement runs on.
func (w *FolkWalker) Queue() *sim.Queue { return w.cfg.Queue }

// Position returns the NPC's current position.
func (w *FolkWalker) Position() location.Location {
	x, y, z := w.Folk.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// Moving reports whether the NPC is walking.
func (w *FolkWalker) Moving() bool { return w.move.Moving() }

// MoveToLocation starts a walk to target and shows it to observers.
func (w *FolkWalker) MoveToLocation(target location.Location) (event.Move, error) {
	if w.MovementDisabled() {
		return event.Move{}, fmt.Errorf("npc %d: movement disabled", w.ObjectID())
	}
	return w.ctl.MoveToLocationEvent(target)
}

// SayNPCString shows observers a route node's chat line; an unmapped id
// says nothing.
func (w *FolkWalker) SayNPCString(id int) {
	text, ok := npcstring.Text(int32(id))
	if !ok {
		return
	}
	w.emit(event.NpcSay{NpcID: w.Instance.Template.TemplateID, Text: text})
}

// SocialAction shows observers a route node's social animation.
func (w *FolkWalker) SocialAction(id int) {
	w.emit(event.SocialAction{ID: int32(id)})
}
