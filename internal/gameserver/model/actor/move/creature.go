// Package move models a creature's requested movement state.
package move

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// FollowMode identifies the active follow task flavor.
type FollowMode uint8

const (
	// FollowNone means no follow task is active.
	FollowNone FollowMode = iota
	// FollowFriendly reevaluates a non-combat follow target every second.
	FollowFriendly
	// FollowOffensive reevaluates a combat follow target twice per second.
	FollowOffensive
)

// PositionUpdateInterval is the movement correction cadence.
const PositionUpdateInterval = 100 * time.Millisecond

const worldZMax = 16410

// TargetSnapshot is the target state a follow tick needs. Build Known from
// the current known-list relationship before calling FollowTick.
type TargetSnapshot struct {
	ObjectID        int32
	Position        location.Location
	CollisionRadius float64
	Known, InBoat   bool
}

// CreatureMove holds movement state owned and updated by one caller.
//
// origin is the actor's current server-authoritative position. mu guards
// every mutable field below, since an accepted MoveToLocation arms a timer
// on the owner's queue that advances origin and fires the arrived hook
// independently of the caller.
//
// A single request may resolve into multiple segments: when the straight
// line is blocked, route resolution produces a sequence of waypoints
// (segments) the actor walks in order. The current segment's destination
// lives in destination; the remaining ones queue in waypoints. The arrived
// hook fires only when the final segment completes, not once per segment.
//
// The arrival timer preserves progress when no position-update task is
// wired. When it is wired, UpdatePosition advances origin at the fixed
// movement correction cadence and may complete the move first. A blocked
// interpolation tick that still has queued waypoints reseeds the next
// remaining leg instead of stopping; the blocked hook fires only once the
// route is exhausted, including when an earlier tick on that same request
// was blocked.
type CreatureMove struct {
	geo          Geo
	waterSurface func(location.Location, int) (int, bool)

	mu                   sync.Mutex
	origin, destination  location.Location
	waypoints            []location.Location
	accurateX, accurateY float64
	speed                float64
	moving               bool
	routeBlocked         bool
	followTarget         int32
	followOffset         int
	followMode           FollowMode
	owner                moveOwner
	timer                *sim.Timer
	moveSeq              uint64
	queue                *sim.Queue
}

// moveOwner is the controller a CreatureMove reports its movement milestones
// to, each after the move's lock is released.
type moveOwner interface {
	// arrived runs once an accepted move reaches its destination.
	arrived()
	// blocked runs when an in-flight move stops because a geodata path
	// closed and no remaining geopath waypoint can continue the route.
	blocked()
	// segmentAdvanced runs each time a multi-segment move advances to the
	// next queued waypoint, with the newly active segment. It does not run
	// for the first segment (MoveToLocation already returns it) or for the
	// final segment's completion (arrived does).
	segmentAdvanced(event.Move)
}

// NewCreatureMove builds movement state at origin with a non-negative ground
// speed. Zero is a valid, stationary speed (e.g. an immobile scripted NPC) —
// MoveToLocation rejects any actual movement request once speed is zero.
func NewCreatureMove(origin location.Location, speed float64, geo Geo) (*CreatureMove, error) {
	state := &CreatureMove{}
	if err := state.Init(origin, speed, geo); err != nil {
		return nil, err
	}
	return state, nil
}

// Init initializes zero movement state embedded in a live actor. Do not call
// it after the state is exposed to callers.
func (m *CreatureMove) Init(origin location.Location, speed float64, geo Geo) error {
	if geo == nil {
		return errors.New("move: nil geodata")
	}
	if speed < 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		return errors.New("move: speed must not be negative")
	}
	m.origin = origin
	m.destination = origin
	m.accurateX = float64(origin.X)
	m.accurateY = float64(origin.Y)
	m.speed = speed
	m.geo = geo
	return nil
}

// SetSpeed changes the speed used by subsequent movement updates.
func (m *CreatureMove) SetSpeed(speed float64) {
	if speed < 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		return
	}
	m.mu.Lock()
	m.speed = speed
	m.mu.Unlock()
}

// setOwner records the controller this move reports milestones to. With no
// owner (the default) every milestone is a no-op.
func (m *CreatureMove) setOwner(owner moveOwner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.owner = owner
}

// SetQueue runs this movement's arrival callbacks as tasks on q, the owning
// actor's queue. A move with a positive duration needs one.
func (m *CreatureMove) SetQueue(q *sim.Queue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue = q
}

// Queue returns the queue SetQueue installed.
func (m *CreatureMove) Queue() *sim.Queue {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queue
}

// SetWaterSurface records the query used to cap underwater movement at a
// water surface. A nil query leaves ground movement capped at the world max.
func (m *CreatureMove) SetWaterSurface(query func(location.Location, int) (int, bool)) {
	m.mu.Lock()
	m.waterSurface = query
	m.mu.Unlock()
}

// CanMoveTo reports whether a straight-line geodata walk from the current
// origin reaches target.
func (m *CreatureMove) CanMoveTo(target location.Location) bool {
	m.mu.Lock()
	origin := m.origin
	geo := m.geo
	m.mu.Unlock()
	return geo.CanMove(origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z)
}

// Position returns the actor's current server-authoritative position.
func (m *CreatureMove) Position() location.Location {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.origin
}

// SetPosition records the actor's current server-authoritative position.
func (m *CreatureMove) SetPosition(position location.Location) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.origin = position
	m.accurateX = float64(position.X)
	m.accurateY = float64(position.Y)
	if m.destination == position {
		// Position reports arrival at the active destination; any queued
		// segments are dropped, since the caller is the authoritative
		// source of position corrections.
		m.waypoints = nil
		m.moving = false
	}
}

// ValidLocation resolves a destination against this creature's movement
// geodata without starting an ordinary move.
func (m *CreatureMove) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	m.mu.Lock()
	geo := m.geo
	m.mu.Unlock()
	return geo.ValidLocation(ox, oy, oz, tx, ty, tz)
}

// Height is the geodata floor at (x, y) using z as the search hint.
func (m *CreatureMove) Height(x, y, z int) int16 {
	m.mu.Lock()
	geo := m.geo
	m.mu.Unlock()
	return geo.Height(x, y, z)
}

// Walkable reports whether (x, y, z) is open geodata in every direction.
func (m *CreatureMove) Walkable(x, y, z int) bool {
	m.mu.Lock()
	geo := m.geo
	m.mu.Unlock()
	return geo.Walkable(x, y, z)
}

// MoveToLocation is the outcome-free convenience form of
// MoveToLocationWithPathOutcome, retained for tests and embedded movement
// state. Production chase/wander/walker accounting goes through Controller.
func (m *CreatureMove) MoveToLocation(target location.Location) (event.Move, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, _, err := m.moveToLocationLocked(target)
	return ev, err
}

// MoveToLocationWithPathOutcome behaves like MoveToLocation and also reports
// how geodata resolved the route, so callers can count blocked pathfinding
// attempts the same way a successful routed search clears that streak.
func (m *CreatureMove) MoveToLocationWithPathOutcome(target location.Location) (event.Move, pathFindResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveToLocationLocked(target)
}

type pathFindResult int

const (
	pathDirect pathFindResult = iota
	pathRouted
	pathFailed
)

func (m *CreatureMove) moveToLocationLocked(target location.Location) (event.Move, pathFindResult, error) {
	target.Z = int(m.geo.Height(target.X, target.Y, target.Z))
	// Retargeting an in-flight walk keeps a mid-route block sticky through
	// the new destination. Only a fresh request (not currently moving)
	// clears it.
	if !m.moving {
		m.routeBlocked = false
	}

	// Same-cell requests complete on the next movement tick.
	if target.X == m.origin.X && target.Y == m.origin.Y {
		m.waypoints = nil
		m.destination = target
		m.accurateX = float64(m.origin.X)
		m.accurateY = float64(m.origin.Y)
		m.moving = true
		m.rescheduleLocked(PositionUpdateInterval)
		return event.Move{Origin: m.origin, Destination: target, Speed: m.speed}, pathDirect, nil
	}

	if m.speed == 0 {
		return event.Move{}, pathDirect, errors.New("move: actor cannot move at zero speed")
	}

	destination, waypoints, outcome := m.resolvePathLocked(target)

	distance := math.Hypot(float64(destination.X)-float64(m.origin.X), float64(destination.Y)-float64(m.origin.Y))
	ticks := math.Ceil(distance / (m.speed / 10))
	const tickDuration = 100 * time.Millisecond
	if math.IsNaN(ticks) || ticks > float64(time.Duration(1<<63-1)/tickDuration) {
		return event.Move{}, outcome, errors.New("move: duration exceeds limit")
	}
	duration := time.Duration(ticks) * tickDuration
	origin := m.origin
	m.accurateX = float64(origin.X)
	m.accurateY = float64(origin.Y)
	m.destination = destination
	m.waypoints = waypoints
	m.moving = true
	if duration == 0 {
		m.rescheduleLocked(PositionUpdateInterval)
	} else {
		m.rescheduleLocked(duration)
	}

	return event.Move{
		Origin:      origin,
		Destination: destination,
		Speed:       m.speed,
		Duration:    duration,
	}, outcome, nil
}

// resolvePathLocked applies the three-tier route resolution a move request
// uses: a straight-line reachability check (tier 1), a routed search around
// the obstacle (tier 2), and a partial-progress last reachable point when
// the route cannot complete (tier 3). The returned destination is the
// first segment to walk; waypoints holds the remaining segments, if any.
//
// origin and target carry geodata-snapped Z.
func (m *CreatureMove) resolvePathLocked(target location.Location) (location.Location, []location.Location, pathFindResult) {
	if m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z) {
		return target, nil, pathDirect
	}

	if path, ok := m.geo.FindPath(m.origin, target); ok && len(path) >= 2 {
		// The pathfinder returns every corner plus the final target cell,
		// omitting the origin. Treat the first entry as the active segment
		// and queue the rest for per-segment advancement.
		destination := path[0]
		var tail []location.Location
		if len(path) > 1 {
			tail = make([]location.Location, len(path)-1)
			copy(tail, path[1:])
		}
		return destination, tail, pathRouted
	}

	fallback := m.geo.ValidLocation(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z)
	return fallback, nil, pathFailed
}

// rescheduleLocked cancels any pending arrival timer and, for a positive
// duration, starts a new one that advances origin to destination and fires
// the arrived hook once it elapses. Callers hold mu.
func (m *CreatureMove) rescheduleLocked(duration time.Duration) {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if duration <= 0 {
		return
	}
	m.moveSeq++
	seq := m.moveSeq
	m.timer = m.queue.After(duration, func() { m.onArrive(seq) })
}

func (m *CreatureMove) onArrive(seq uint64) {
	m.mu.Lock()
	if seq != m.moveSeq || !m.moving {
		m.mu.Unlock()
		return
	}
	action := m.finishLocked()
	m.mu.Unlock()

	if action != nil {
		action()
	}
}

// finishLocked completes the just-elapsed segment and either advances to the
// next queued waypoint or, once the queue is empty, stops the move. It
// returns a callback for the caller to invoke after unlocking: the arrived
// hook when the move is fully done, or the segment-advanced hook (bound to
// the newly active segment's event) when another waypoint remains. Callers
// hold mu.
func (m *CreatureMove) finishLocked() func() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	// Snap to the just-completed segment's destination.
	m.destination.Z = min(m.destination.Z, m.maxZLocked())
	m.origin = m.destination
	m.accurateX = float64(m.destination.X)
	m.accurateY = float64(m.destination.Y)

	// Advance through any remaining waypoints. Zero-distance segments are
	// skipped silently; a positive-distance segment becomes the active
	// destination with a freshly scheduled arrival timer. Scheduling a new
	// timer advances moveSeq, so the previous segment's onArrive callback
	// (if still in flight) is ignored.
	const tickDuration = 100 * time.Millisecond
	for len(m.waypoints) > 0 {
		next := m.waypoints[0]
		m.waypoints = m.waypoints[1:]
		distance := math.Hypot(float64(next.X)-float64(m.origin.X), float64(next.Y)-float64(m.origin.Y))
		ticks := math.Ceil(distance / (m.speed / 10))
		if math.IsNaN(ticks) || ticks > float64(time.Duration(1<<63-1)/tickDuration) {
			// Unrepresentable next-segment duration: stop here, drop tail.
			m.waypoints = nil
			m.moving = false
			return m.arrivalHookLocked()
		}
		duration := time.Duration(ticks) * tickDuration
		if duration <= 0 {
			// Zero-distance segment: snap forward and continue.
			m.destination = next
			m.origin = next
			m.accurateX = float64(next.X)
			m.accurateY = float64(next.Y)
			continue
		}
		m.destination = next
		m.moving = true
		m.rescheduleLocked(duration)
		ev := m.currentEventLocked()
		owner := m.owner
		if owner == nil {
			return nil
		}
		return func() { owner.segmentAdvanced(ev) }
	}

	m.moving = false
	return m.arrivalHookLocked()
}

// UpdatePosition advances one in-flight movement by step and reports the
// current movement event to broadcast. It returns false after the move has
// already stopped or reaches its final destination. Reaching an intermediate
// waypoint — including when the current leg is blocked and a later waypoint
// remains — returns the next segment's event with true.
func (m *CreatureMove) UpdatePosition(step time.Duration) (event.Move, bool) {
	m.mu.Lock()
	if !m.moving {
		m.mu.Unlock()
		return event.Move{}, false
	}
	if step <= 0 {
		ev := m.currentEventLocked()
		m.mu.Unlock()
		return ev, true
	}

	maxZ := m.maxZLocked()
	m.destination.Z = min(int(m.geo.Height(m.destination.X, m.destination.Y, m.destination.Z)), maxZ)
	dx := float64(m.destination.X) - m.accurateX
	dy := float64(m.destination.Y) - m.accurateY
	left := math.Hypot(dx, dy)
	passed := m.speed * step.Seconds()
	nextAccurateX, nextAccurateY := m.accurateX, m.accurateY
	next := m.destination
	if left != 0 && passed < left {
		fraction := passed / left
		nextAccurateX += dx * fraction
		nextAccurateY += dy * fraction
		next.X = int(nextAccurateX)
		next.Y = int(nextAccurateY)
		next.Z = min(int(m.geo.Height(next.X, next.Y, m.origin.Z+2*block.CellHeight)), maxZ)
	}
	if !m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, next.X, next.Y, next.Z) {
		m.routeBlocked = true
		ok, action := m.startNextWaypointLocked()
		if !ok {
			action = m.stopBlockedLocked()
			m.mu.Unlock()
			if action != nil {
				action()
			}
			return event.Move{}, false
		}
		ev := m.currentEventLocked()
		m.mu.Unlock()
		if action != nil {
			action()
		}
		return ev, true
	}
	if left == 0 || passed >= left {
		action := m.finishLocked()
		if !m.moving {
			m.mu.Unlock()
			if action != nil {
				action()
			}
			return event.Move{}, false
		}
		// Advanced to the next waypoint segment; report it and fire the
		// segment-advanced hook (if any) so the client is told about the new
		// leg the same tick the server itself commits to it.
		ev := m.currentEventLocked()
		m.mu.Unlock()
		if action != nil {
			action()
		}
		return ev, true
	}

	m.accurateX = nextAccurateX
	m.accurateY = nextAccurateY
	m.origin = next
	ev := m.currentEventLocked()
	m.mu.Unlock()
	return ev, true
}

func (m *CreatureMove) maxZLocked() int {
	if m.waterSurface == nil {
		return worldZMax
	}
	if surface, ok := m.waterSurface(m.origin, int(m.geo.Height(m.origin.X, m.origin.Y, m.origin.Z))); ok {
		return surface
	}
	return worldZMax
}

func (m *CreatureMove) arrivalHookLocked() func() {
	if m.owner == nil {
		return nil
	}
	if m.routeBlocked {
		return m.owner.blocked
	}
	return m.owner.arrived
}

func (m *CreatureMove) stopBlockedLocked() func() {
	m.rescheduleLocked(0)
	m.waypoints = nil
	m.moving = false
	if m.owner == nil {
		return nil
	}
	return m.owner.blocked
}

// startNextWaypointLocked starts the next queued geopath leg from the
// current cell without snapping onto the blocked destination. One waypoint
// per tick; a dry queue or a non-positive speed reports that the route is
// exhausted. Callers hold mu.
func (m *CreatureMove) startNextWaypointLocked() (ok bool, action func()) {
	if len(m.waypoints) == 0 || m.speed <= 0 {
		return false, nil
	}
	next := m.waypoints[0]
	m.waypoints = m.waypoints[1:]
	m.accurateX = float64(m.origin.X)
	m.accurateY = float64(m.origin.Y)
	m.destination = next

	distance := math.Hypot(float64(next.X)-float64(m.origin.X), float64(next.Y)-float64(m.origin.Y))
	ticks := math.Ceil(distance / (m.speed / 10))
	const tickDuration = 100 * time.Millisecond
	if math.IsNaN(ticks) || ticks > float64(time.Duration(1<<63-1)/tickDuration) {
		m.waypoints = nil
		m.moving = false
		return false, nil
	}
	duration := time.Duration(ticks) * tickDuration
	if duration <= 0 {
		duration = PositionUpdateInterval
	}
	m.moving = true
	m.rescheduleLocked(duration)
	ev := m.currentEventLocked()
	owner := m.owner
	if owner == nil {
		return true, nil
	}
	return true, func() { owner.segmentAdvanced(ev) }
}

func (m *CreatureMove) currentEventLocked() event.Move {
	ev := event.Move{
		Origin:      m.origin,
		Destination: m.destination,
		Speed:       m.speed,
	}
	if m.followMode == FollowOffensive {
		ev.FollowTarget = m.followTarget
		ev.FollowOffset = m.followOffset
	}
	return ev
}

// CancelMove stops any pending arrival timer and leaves the actor at its
// current position, without changing follow state.
func (m *CreatureMove) CancelMove() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rescheduleLocked(0)
	m.waypoints = nil
	m.moving = false
}

// Moving reports whether the current request has non-zero ground distance
// still in flight.
func (m *CreatureMove) Moving() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moving
}

// Destination returns the target of the last accepted movement request.
func (m *CreatureMove) Destination() location.Location {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.destination
}

// StartFriendlyFollow starts a friendly follow task for targetID.
func (m *CreatureMove) StartFriendlyFollow(targetID int32, offset int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.followMode = FollowFriendly
	m.followTarget = targetID
	m.followOffset = offset
}

// StartOffensiveFollow starts an offensive follow task for targetID.
func (m *CreatureMove) StartOffensiveFollow(targetID int32, offset int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.followMode = FollowOffensive
	m.followTarget = targetID
	m.followOffset = offset
}

// CancelFollow clears any active follow task.
func (m *CreatureMove) CancelFollow() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.followMode = FollowNone
	m.followTarget = 0
	m.followOffset = 0
}

// Following reports whether a follow task is active.
func (m *CreatureMove) Following() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.followMode != FollowNone
}

// FollowMode returns the active follow mode.
func (m *CreatureMove) FollowMode() FollowMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.followMode
}

// FollowInterval returns how often the active follow task should be ticked.
func (m *CreatureMove) FollowInterval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.followIntervalLocked()
}

func (m *CreatureMove) followIntervalLocked() time.Duration {
	switch m.followMode {
	case FollowFriendly:
		return time.Second
	case FollowOffensive:
		return 500 * time.Millisecond
	default:
		return 0
	}
}

// FollowTick reevaluates the active follow target and starts movement when
// the target is still known and outside the collision-adjusted follow range.
// Tests are the only callers. Path-outcome accounting is not applied here;
// production chase goes through Controller.maybeStartFollow.
func (m *CreatureMove) FollowTick(target TargetSnapshot, actorRadius float64) (event.Move, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.followMode == FollowNone || target.ObjectID != m.followTarget || !target.Known {
		return event.Move{}, false, nil
	}
	if m.followMode == FollowFriendly && target.InBoat {
		return event.Move{}, false, nil
	}

	if m.origin.In2DRadius(target.Position, followRange(m.followOffset, actorRadius, target.CollisionRadius)) {
		return event.Move{}, false, nil
	}

	followMode := m.followMode
	followOffset := m.followOffset
	ev, _, err := m.moveToLocationLocked(target.Position)
	if err != nil {
		return event.Move{}, false, err
	}
	if followMode == FollowOffensive {
		ev.FollowTarget = target.ObjectID
		ev.FollowOffset = followOffset
	}
	return ev, true, nil
}

func followRange(offset int, actorRadius, targetRadius float64) int {
	return int(float64(offset) + actorRadius + targetRadius)
}
