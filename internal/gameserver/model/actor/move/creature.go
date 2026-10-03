// Package move models a creature's requested movement state.
package move

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
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

// walkStartUpdates is how many position updates at the start of a move use
// the start speed SetSpeeds records (a player's walk speed).
const walkStartUpdates = 5

// maxTravelTicks is the largest position-update count a duration can hold.
const maxTravelTicks = float64(time.Duration(1<<63-1) / PositionUpdateInterval)

// Pawn is the target of a pawn walk: the walk toward a creature pawn ends
// once its last leg brings the walker within the walk's offset of the pawn's
// current position, and the walk toward any other pawn (a static object)
// runs onto the pawn's point. A tracking pawn walk
// (MoveToPawnWithPathOutcome) also re-aims at that position on every
// position update of its last leg.
type Pawn interface {
	ObjectID() int32
	Kind() actor.Kind
	Position() (x, y, z int)
}

// MoveType is how a mover travels: on the ground, swimming or flying.
type MoveType uint8

const (
	// MoveGround walks on the geodata floor and measures distances on the
	// ground plane.
	MoveGround MoveType = iota
	// MoveSwim moves through a water zone.
	MoveSwim
	// MoveFly moves through the air.
	MoveFly
)

// waterDepthMin is how far below a water zone's surface the floor must lie
// for the surface to cap a mover's height.
const waterDepthMin = 20

const worldZMax = 16410

// flyCorridorHeight is the corridor height a swimming or flying player's
// straight-line and partial-progress fly queries test.
const flyCorridorHeight = 32

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
// every mutable field below. The arrival timer runs on the owner's queue,
// but another actor's effect stops the move synchronously from its own
// queue (AbortAll or StopMove).
//
// A single request may resolve into multiple segments: when the straight
// line is blocked, route resolution produces a sequence of waypoints
// (segments) the actor walks in order. The current segment's destination
// lives in destination; the remaining ones queue in waypoints. The arrived
// hook fires only when the final segment completes, not once per segment.
//
// A tracking pawn walk (a player's) never path-finds: it heads straight for
// its pawn, and a line that closes ends it blocked on the position update
// that meets it. A chase pawn walk (an NPC's or a summon's) resolves its route
// like a walk to a fixed point and never re-aims; only its stop follows the
// pawn.
//
// A mover standing in a water zone swims, and one SetFlying marks flies (a
// swimmer counts as swimming even while flying). A mover UseZoneSwim opts in
// (a player) swims instead only while SetSwimming says its zones hold it in
// water, which may lag behind where it stands. A swimming or flying mover
// measures its legs, steps and pawn stops in 3D, moves its height straight
// toward the destination's instead of along the geodata floor, and is never
// stopped by a closed geodata line under way.
//
// A mover given SetSpeeds (a player) steps by the player rules: each update
// measures its delta from the current cell and rounds the next one, advances
// by the queue-clock time passed since the move started or the last update
// (a retarget catch-up included), and retargeting a walk in flight first
// advances it by the time passed since the last update. Every update
// measures from the one before it, never from the shared ticker's schedule,
// so a late update walks the extra time once and the next one only what is
// left: the walk always covers the time that passed.
//
// The arrival timer preserves progress when no position-update task is
// wired. When it is wired, the position updates advance origin on the
// movement correction ticks and may complete the move first. A timer that
// elapses on the last leg of a pawn walk runs the position updates it stands
// for, so it ends the walk where they would, whichever of the two runs
// first. A blocked interpolation tick that still has queued waypoints
// reseeds the next remaining leg instead of stopping; the blocked hook fires
// only once the route is exhausted, including when an earlier tick on that
// same request was blocked.
type CreatureMove struct {
	geo   Geo
	water func(location.Location) (int, bool)

	mu                              sync.Mutex
	origin, destination             location.Location
	waypoints                       []location.Location
	accurateX, accurateY, accurateZ float64
	flying                          bool
	// zoneSwim marks a mover whose swimming follows swimming, set from its
	// zone membership, instead of the water query.
	zoneSwim, swimming bool
	// timedType is the move type the active leg's arrival was timed with;
	// a position update that changes the type re-times the leg.
	timedType    MoveType
	speed        float64
	moving       bool
	routeBlocked bool
	followTarget int32
	followOffset int
	followMode   FollowMode
	// pawn is the target a pawn walk stops at, nil for a walk to a fixed
	// point; pawnStop is how far short of it the walk stops, its offset for
	// a creature pawn and 0 for any other; pawnTracks marks a tracking pawn
	// walk; pawnGen tells one pawn walk from the next.
	pawn       Pawn
	pawnStop   int
	pawnTracks bool
	pawnGen    uint64
	// startSpeed, when hasStartSpeed, is the speed the first
	// walkStartUpdates position updates of a move use; updates counts the
	// position updates since the move started and restarts only once the
	// move stops, not when it is retargeted.
	startSpeed    float64
	hasStartSpeed bool
	updates       int
	// lastUpdate is when the last position update, retarget catch-up or
	// the move start ran on the queue clock. Player movers only.
	lastUpdate time.Time
	// catchUps counts the retarget catch-ups that stepped the mover (see
	// catchUpLocked).
	catchUps uint64
	owner    moveOwner
	timer    *sim.Timer
	// timedSteps is how many position updates the pending arrival timer
	// stands for.
	timedSteps int
	moveSeq    uint64
	queue      *sim.Queue
	// routeBufs back routed waypoints so a repeated geopath search reuses
	// their storage. waypoints may alias routeBufs[routeBuf] while a new
	// request searches into the other one, which becomes current only when
	// that request commits its route, so a search never overwrites a route
	// still being walked.
	routeBufs [2][]location.Location
	routeBuf  int
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
	// pawnStepped runs when the arrival timer of a tracking pawn walk steps
	// the walker from from to to, before the walk's milestone.
	pawnStepped(from, to location.Location)
	// knowsPawn reports whether the actor still knows the pawn its walk
	// heads for; the arrival timer ends the walk of a pawn it does not.
	knowsPawn(Pawn) bool
	// mayCatchUp reports whether a request heading for pawn (nil for a
	// fixed point) may run the retarget catch-up of a walk in flight: the
	// actor is visible and knows pawn.
	mayCatchUp(Pawn) bool
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
	m.setAccurateLocked(origin)
	m.speed = speed
	m.geo = geo
	return nil
}

// setAccurateLocked reseeds the accurate position at the cell at. Callers
// hold mu or own the state exclusively.
func (m *CreatureMove) setAccurateLocked(at location.Location) {
	m.accurateX = float64(at.X)
	m.accurateY = float64(at.Y)
	m.accurateZ = float64(at.Z)
}

// Speed returns the speed movement updates currently use.
func (m *CreatureMove) Speed() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.speed
}

// SetSpeed changes the speed used by subsequent movement updates. An
// in-flight leg keeps going from where it stands, its arrival re-timed for
// the distance left at the new speed; at zero speed it stalls without an
// arrival until the speed returns.
func (m *CreatureMove) SetSpeed(speed float64) {
	if !validSpeed(speed) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.speed == speed {
		return
	}
	m.speed = speed
	m.retimeLocked()
}

// SetSpeeds is SetSpeed for a player's mover, whose moves start slower: the
// first walkStartUpdates position updates of each move advance at
// startSpeed, the rest at speed. The count restarts only once a move stops,
// so retargeting a walk in flight keeps its pace. It also makes the mover
// step by the player rules (see CreatureMove).
func (m *CreatureMove) SetSpeeds(speed, startSpeed float64) {
	if !validSpeed(speed) || !validSpeed(startSpeed) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.speed == speed && m.hasStartSpeed && m.startSpeed == startSpeed {
		return
	}
	m.speed = speed
	m.startSpeed = startSpeed
	m.hasStartSpeed = true
	m.retimeLocked()
}

// playerStepsLocked reports that the mover steps by the player rules, which
// SetSpeeds opts it into. Callers hold mu.
func (m *CreatureMove) playerStepsLocked() bool {
	return m.hasStartSpeed
}

// nowLocked reads the queue clock, or the zero time with no queue. Callers
// hold mu.
func (m *CreatureMove) nowLocked() time.Time {
	if m.queue == nil {
		return time.Time{}
	}
	return m.queue.Now()
}

// playerPassed is how far a player's position update covering elapsed
// advances at speed: the elapsed whole milliseconds, at least one, of the
// speed per second.
func playerPassed(speed float64, elapsed time.Duration) float64 {
	ms := elapsed.Milliseconds()
	if ms <= 0 {
		ms = 1
	}
	return speed / (1000 / float64(ms))
}

// roundHalfUp rounds v to the nearest integer, halves toward +Inf.
func roundHalfUp(v float64) int {
	return int(math.Floor(v + 0.5))
}

func validSpeed(speed float64) bool {
	return speed >= 0 && !math.IsNaN(speed) && !math.IsInf(speed, 0)
}

// retimeLocked re-times an in-flight leg's arrival for the distance left at
// the current speeds; at zero speed it stalls without an arrival until the
// speed returns. Callers hold mu.
func (m *CreatureMove) retimeLocked() {
	if !m.moving || m.queue == nil {
		return
	}
	if m.speed == 0 {
		m.rescheduleLocked(0)
		return
	}
	if delay := m.arrivalDelayLocked(); delay > 0 {
		m.rescheduleLocked(delay)
	}
}

// updateSpeedLocked is the speed the current position update advances at.
// Callers count the update first.
func (m *CreatureMove) updateSpeedLocked() float64 {
	if m.hasStartSpeed && m.updates <= walkStartUpdates {
		return m.startSpeed
	}
	return m.speed
}

// travelTicksLocked is how many position updates cover distance from the
// update count reached so far, the remaining start-speed updates first.
// It is NaN or +Inf when the speeds cannot cover it. Callers hold mu.
func (m *CreatureMove) travelTicksLocked(distance float64) float64 {
	var ticks float64
	if m.hasStartSpeed && m.updates < walkStartUpdates {
		startTicks := float64(walkStartUpdates - m.updates)
		startStep := m.startSpeed / 10
		if startStep > 0 && distance <= startTicks*startStep {
			return math.Ceil(distance / startStep)
		}
		ticks = startTicks
		distance -= startTicks * startStep
	}
	return ticks + math.Ceil(distance/(m.speed/10))
}

// arrivalDelayLocked is how long the rest of the active leg takes from the
// accurate position, at least one position update; on the last leg of a
// tracking pawn walk only up to where it stops short of the destination,
// plus the update whose step brings it strictly within the offset of a
// creature pawn, so the timer falls due past the update that ends the walk.
// It is 0 when the leg cannot finish at the current speeds. It records the
// move type the leg is timed with. Callers hold mu.
func (m *CreatureMove) arrivalDelayLocked() time.Duration {
	m.timedType = m.moveTypeLocked()
	distance := m.leftLocked(m.timedType, m.accurateX, m.accurateY)
	var within float64
	if m.onTrackedPawnLegLocked() {
		distance = max(distance-float64(m.pawnStop), 0)
		if m.pawnStop > 0 {
			within = 1
		}
	}
	ticks := m.travelTicksLocked(distance) + within
	if math.IsNaN(ticks) || ticks > maxTravelTicks {
		return 0
	}
	return max(time.Duration(ticks)*PositionUpdateInterval, PositionUpdateInterval)
}

// onLastPawnLegLocked reports that the active leg is the last one of a pawn
// walk, the leg that stops at the pawn. Callers hold mu.
func (m *CreatureMove) onLastPawnLegLocked() bool {
	return m.moving && m.pawn != nil && len(m.waypoints) == 0
}

// onTrackedPawnLegLocked reports that the active leg is the last one of a
// tracking pawn walk, the leg that re-aims at the pawn. Callers hold mu.
func (m *CreatureMove) onTrackedPawnLegLocked() bool {
	return m.onLastPawnLegLocked() && m.pawnTracks
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

// SetWaterSurface records the query for the water zone at a position: its
// surface level, and whether the position lies in one. A mover standing in a
// water zone swims, capped at the surface while the floor lies deeper than
// waterDepthMin below it. A nil query leaves the mover on the ground, capped
// at the world max.
func (m *CreatureMove) SetWaterSurface(query func(location.Location) (int, bool)) {
	m.mu.Lock()
	m.water = query
	m.mu.Unlock()
}

// SetFlying records whether the mover flies (a wyvern rider).
func (m *CreatureMove) SetFlying(flying bool) {
	m.mu.Lock()
	m.flying = flying
	m.mu.Unlock()
}

// UseZoneSwim makes the mover swim only while SetSwimming says so, instead
// of whenever the water query finds it in a water zone. The water query then
// only caps a flying mover at the surface. Call it before the mover is
// exposed.
func (m *CreatureMove) UseZoneSwim() {
	m.mu.Lock()
	m.zoneSwim = true
	m.mu.Unlock()
}

// SetSwimming records whether the mover's zones hold it in water, for a mover
// UseZoneSwim opted in. A leg in flight is re-timed for the distance left
// under the new move type.
func (m *CreatureMove) SetSwimming(swimming bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.swimming == swimming {
		return
	}
	m.swimming = swimming
	if m.moving && m.moveTypeLocked() != m.timedType {
		m.retimeLocked()
	}
}

// MoveType reports how the mover travels from where it stands now.
func (m *CreatureMove) MoveType() MoveType {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveTypeLocked()
}

// moveTypeLocked is the mover's MoveType at its origin: swimming in a water
// zone (for a UseZoneSwim mover, while SetSwimming says so), else flying
// when SetFlying marks it, else on the ground. Callers hold mu.
func (m *CreatureMove) moveTypeLocked() MoveType {
	if m.zoneSwim {
		return m.flagTypeLocked()
	}
	if m.water != nil {
		if _, ok := m.water(m.origin); ok {
			return MoveSwim
		}
	}
	if m.flying {
		return MoveFly
	}
	return MoveGround
}

// flagTypeLocked is a UseZoneSwim mover's MoveType: swimming while
// SetSwimming says so, else flying when SetFlying marks it, else on the
// ground. Callers hold mu.
func (m *CreatureMove) flagTypeLocked() MoveType {
	switch {
	case m.swimming:
		return MoveSwim
	case m.flying:
		return MoveFly
	}
	return MoveGround
}

// span is the length of the leg (dx, dy, dz) a mover of type t covers: on
// the ground plane for a ground mover, through space otherwise.
func span(t MoveType, dx, dy, dz float64) float64 {
	if t == MoveGround {
		return math.Hypot(dx, dy)
	}
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// between is the length of the leg from a to b a mover of type t covers.
func between(t MoveType, a, b location.Location) float64 {
	return span(t, float64(b.X)-float64(a.X), float64(b.Y)-float64(a.Y), float64(b.Z)-float64(a.Z))
}

// leftLocked is how far a mover of type t standing at (x, y) and the
// origin's height has left to the destination. Callers hold mu.
func (m *CreatureMove) leftLocked(t MoveType, x, y float64) float64 {
	return span(t, float64(m.destination.X)-x, float64(m.destination.Y)-y, float64(m.destination.Z)-float64(m.origin.Z))
}

// inRadius reports whether a lies strictly within radius of b as a mover of
// type t measures it: on the ground plane for a ground mover, in 3D
// otherwise.
func inRadius(t MoveType, a, b location.Location, radius int) bool {
	if t == MoveGround {
		return a.In2DRadius(b, radius)
	}
	return a.In3DRadius(b, radius)
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
	m.setAccurateLocked(position)
	if m.destination == position {
		// Position reports arrival at the active destination; any queued
		// segments are dropped, since the caller is the authoritative
		// source of position corrections.
		m.waypoints = nil
		m.moving = false
		m.pawn = nil
	}
}

// ValidLocation resolves a destination against this creature's movement
// geodata without starting an ordinary move. Movement never initialized with
// geodata applies no correction and returns the destination unchanged.
func (m *CreatureMove) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	m.mu.Lock()
	geo := m.geo
	m.mu.Unlock()
	if geo == nil {
		return location.Location{X: tx, Y: ty, Z: tz}
	}
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
	catchUp := m.mayCatchUp(nil)
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, _, err := m.moveToLocationLocked(target, nil, 0, false, catchUp)
	return ev, err
}

// MoveToLocationWithPathOutcome behaves like MoveToLocation and also reports
// how geodata resolved the route, so callers can count blocked pathfinding
// attempts the same way a successful routed search clears that streak.
func (m *CreatureMove) MoveToLocationWithPathOutcome(target location.Location) (event.Move, pathFindResult, error) {
	catchUp := m.mayCatchUp(nil)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveToLocationLocked(target, nil, 0, false, catchUp)
}

// MoveToPawnWithPathOutcome starts a tracking pawn walk straight toward
// pawn's current position: on its last leg every position update re-aims it
// at where pawn stands then, and the walk ends once within offset of a
// creature pawn (2D on the ground, 3D swimming or flying), or on the point of
// any other pawn. It otherwise reports like MoveToLocationWithPathOutcome.
func (m *CreatureMove) MoveToPawnWithPathOutcome(pawn Pawn, offset int) (event.Move, pathFindResult, error) {
	x, y, z := pawn.Position()
	catchUp := m.mayCatchUp(pawn)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveToLocationLocked(location.Location{X: x, Y: y, Z: z}, pawn, offset, true, catchUp)
}

// ChasePawnWithPathOutcome starts a chase pawn walk toward pawn's current
// position, routed like MoveToLocationWithPathOutcome: the destination stays
// where pawn stood at the request, and the walk ends on its last leg at the
// first position within offset of where a creature pawn stands then (2D on
// the ground, 3D swimming or flying).
func (m *CreatureMove) ChasePawnWithPathOutcome(pawn Pawn, offset int) (event.Move, pathFindResult, error) {
	x, y, z := pawn.Position()
	catchUp := m.mayCatchUp(pawn)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveToLocationLocked(location.Location{X: x, Y: y, Z: z}, pawn, offset, false, catchUp)
}

type pathFindResult int

const (
	pathDirect pathFindResult = iota
	pathRouted
	pathFailed
)

// mayCatchUp reports whether a request heading for pawn (nil for a fixed
// point) may run the retarget catch-up of a player's walk in flight (see
// moveOwner.mayCatchUp). It reads the owner's answer without holding mu, as
// that may take other actors' locks.
func (m *CreatureMove) mayCatchUp(pawn Pawn) bool {
	m.mu.Lock()
	owner := m.owner
	retarget := m.moving && m.playerStepsLocked()
	m.mu.Unlock()
	return !retarget || owner == nil || owner.mayCatchUp(pawn)
}

// moveToLocationLocked starts a request toward target. catchUp lets a
// player's walk in flight first run its retarget catch-up. Callers hold mu.
func (m *CreatureMove) moveToLocationLocked(target location.Location, pawn Pawn, offset int, tracks, catchUp bool) (event.Move, pathFindResult, error) {
	moveType := m.moveTypeLocked()
	if moveType == MoveGround {
		target.Z = int(m.geo.Height(target.X, target.Y, target.Z))
	}
	// Retargeting an in-flight walk keeps a mid-route block sticky through
	// the new destination, and the start-speed update count running. Only a
	// fresh request (not currently moving) clears them. A player's walk in
	// flight first advances by the time passed since its last update, unless
	// catchUp holds it where it stands.
	if !m.moving {
		m.routeBlocked = false
		m.updates = 0
	} else if m.playerStepsLocked() && catchUp {
		m.catchUpLocked()
	}
	if m.playerStepsLocked() {
		m.lastUpdate = m.nowLocked()
	}

	// Same-cell requests complete on the next movement tick.
	if target.X == m.origin.X && target.Y == m.origin.Y && (moveType == MoveGround || target.Z == m.origin.Z) {
		m.waypoints = nil
		m.destination = target
		m.setAccurateLocked(m.origin)
		m.setPawnLocked(pawn, offset, tracks)
		m.moving = true
		m.rescheduleLocked(PositionUpdateInterval)
		return event.Move{Origin: m.origin, Destination: target, Speed: m.speed}, pathDirect, nil
	}

	if m.speed == 0 {
		return event.Move{}, pathDirect, errors.New("move: actor cannot move at zero speed")
	}

	// A tracking pawn walk heads straight for the pawn: a closed line ends it
	// blocked on the update that meets it, never routed around.
	destination, outcome := target, pathDirect
	var waypoints []location.Location
	if pawn == nil || !tracks {
		destination, waypoints, outcome = m.resolvePathLocked(target)
	}

	distance := between(moveType, m.origin, destination)
	ticks := m.travelTicksLocked(distance)
	if math.IsNaN(ticks) || ticks > maxTravelTicks {
		return event.Move{}, outcome, errors.New("move: duration exceeds limit")
	}
	duration := time.Duration(ticks) * PositionUpdateInterval
	origin := m.origin
	m.setAccurateLocked(origin)
	m.destination = destination
	m.waypoints = waypoints
	if outcome == pathRouted {
		m.routeBuf ^= 1
	}
	m.setPawnLocked(pawn, offset, tracks)
	m.moving = true
	m.rescheduleLocked(m.arrivalDelayLocked())

	return event.Move{
		Origin:      origin,
		Destination: destination,
		Speed:       m.speed,
		Duration:    duration,
	}, outcome, nil
}

// setPawnLocked makes the request a pawn walk toward pawn, tracking it when
// tracks, or a walk to a fixed point when pawn is nil. Only a creature pawn
// stops the walk offset short of it. Callers hold mu.
func (m *CreatureMove) setPawnLocked(pawn Pawn, offset int, tracks bool) {
	m.pawn = pawn
	m.pawnStop = 0
	if pawn != nil && pawn.Kind().Creature() {
		m.pawnStop = offset
	}
	m.pawnTracks = pawn != nil && tracks
	m.pawnGen++
}

// resolvePathLocked applies the three-tier route resolution a move request
// uses: a straight-line reachability check (tier 1), a routed search around
// the obstacle (tier 2), and a partial-progress last reachable point when
// the route cannot complete (tier 3). The returned destination is the
// first segment to walk; waypoints holds the remaining segments, if any.
//
// origin and target carry geodata-snapped Z.
//
// A player that swims or flies from the origin takes no routed search: a
// flier's line is open by the fly corridor (a swimmer's by the ground line),
// and a line that is not ends at the last point a flier reaches along it.
// Any other mover resolves on the ground whatever its move type.
func (m *CreatureMove) resolvePathLocked(target location.Location) (location.Location, []location.Location, pathFindResult) {
	moveType := MoveGround
	if m.playerStepsLocked() {
		moveType = m.moveTypeLocked()
	}
	var open bool
	if moveType == MoveFly {
		open = m.geo.CanFly(m.origin.X, m.origin.Y, m.origin.Z, flyCorridorHeight, target.X, target.Y, target.Z)
	} else {
		open = m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z)
	}
	if open {
		return target, nil, pathDirect
	}
	if moveType != MoveGround {
		fallback := m.geo.ValidFlyLocation(m.origin.X, m.origin.Y, m.origin.Z, flyCorridorHeight, target.X, target.Y, target.Z)
		return fallback, nil, pathFailed
	}

	if path, ok := m.findPathLocked(target); ok && len(path) >= 2 {
		// The pathfinder returns every corner plus the final target cell,
		// omitting the origin. Treat the first entry as the active segment
		// and queue the rest for per-segment advancement.
		return path[0], path[1:], pathRouted
	}

	fallback := m.geo.ValidLocation(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z)
	return fallback, nil, pathFailed
}

// findPathLocked runs the routed search from origin to target into the
// route buffer the current waypoints do not use, keeping its grown storage
// for the next search. The caller makes that buffer current (routeBuf) only
// when it commits the route. Callers hold mu.
func (m *CreatureMove) findPathLocked(target location.Location) ([]location.Location, bool) {
	next := m.routeBuf ^ 1
	var path []location.Location
	var ok bool
	if g, into := m.geo.(pathIntoFinder); into {
		path, ok = g.FindPathInto(m.routeBufs[next][:0], m.origin, target)
	} else {
		var found []location.Location
		found, ok = m.geo.FindPath(m.origin, target)
		path = append(m.routeBufs[next][:0], found...)
	}
	m.routeBufs[next] = path[:0]
	return path, ok
}

// rescheduleLocked cancels any pending arrival timer and, for a positive
// duration, starts a new one that advances origin to destination and fires
// the arrived hook once it elapses. It always advances moveSeq, so an
// arrival callback that already left the timer (Stop lost the race) is
// dropped even when no new timer replaces it. Callers hold mu.
func (m *CreatureMove) rescheduleLocked(duration time.Duration) {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.moveSeq++
	m.timedSteps = int(duration / PositionUpdateInterval)
	if duration <= 0 {
		return
	}
	seq := m.moveSeq
	m.timer = m.queue.After(duration, func() { m.onArrive(seq) })
}

func (m *CreatureMove) onArrive(seq uint64) {
	// Read the pawn's position, and whether the actor still knows it,
	// before taking mu for the leg's end: either may take other actors'
	// locks. Every change to the leg advances moveSeq.
	m.mu.Lock()
	if seq != m.moveSeq || !m.moving {
		m.mu.Unlock()
		return
	}
	var pawn Pawn
	if m.onLastPawnLegLocked() && (m.pawnTracks || m.pawnStop > 0) {
		pawn = m.pawn
	}
	owner := m.owner
	m.mu.Unlock()
	var pawnAt location.Location
	known := true
	if pawn != nil {
		x, y, z := pawn.Position()
		pawnAt = location.Location{X: x, Y: y, Z: z}
		known = owner == nil || owner.knowsPawn(pawn)
	}

	m.mu.Lock()
	if seq != m.moveSeq || !m.moving {
		m.mu.Unlock()
		return
	}
	var action func()
	switch {
	case pawn != nil && !known:
		// As the update does (abandonPawnWalk), the walk ends where the
		// actor stands, as an arrival.
		action = m.endLocked()
	case pawn != nil:
		action = m.runOutPawnLegLocked(pawnAt)
	default:
		action = m.finishLocked()
	}
	m.mu.Unlock()

	if action != nil {
		action()
	}
}

// runOutPawnLegLocked ends the last leg of a pawn walk whose arrival timer
// elapsed before a position update ended it, where the position updates the
// timer stands for would: it takes their steps one by one from where the
// actor stands, a tracking walk re-aiming each at pawnAt (where the pawn
// stands now), and ends the walk at the first step within the offset of
// pawnAt (2D on the ground, 3D swimming or flying), or on the leg's end.
// When the steps run out first (a tracking walk whose pawn moved away since
// the timer was armed), the walk goes on from there, re-timed for the rest.
//
// A ground walker reads each step's floor from the height the step before
// it landed on, as the updates do, so a run up stairs over a floor stays on
// the stairs; a step whose line is closed ends the walk blocked where the
// last open step left it, as the update meeting it would. A tracking walk
// turns the walker toward its last step, before the walk's milestone.
// Callers hold mu.
func (m *CreatureMove) runOutPawnLegLocked(pawnAt location.Location) func() {
	moveType, maxZ := m.waterLocked()
	player := m.playerStepsLocked()
	var stepFrom location.Location
	stepped := false
	end := false
	for range max(m.timedSteps, 1) {
		m.updates++
		if m.pawnTracks {
			m.destination = pawnAt
		}
		passed := m.updateSpeedLocked() * PositionUpdateInterval.Seconds()
		if player {
			passed = playerPassed(m.updateSpeedLocked(), PositionUpdateInterval)
		}
		next, accurate, reached := m.stepLocked(passed, moveType, maxZ)
		if moveType == MoveGround && !m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, next.X, next.Y, next.Z) {
			m.routeBlocked = true
			var turn func()
			if stepped {
				turn = m.turnLocked(stepFrom)
			}
			return chain(turn, m.stopBlockedLocked())
		}
		if reached {
			accurate = [3]float64{float64(next.X), float64(next.Y), float64(next.Z)}
		}
		stepFrom, stepped = m.origin, true
		m.accurateX, m.accurateY, m.accurateZ = accurate[0], accurate[1], accurate[2]
		m.origin = next
		if reached || (m.pawnStop > 0 && inRadius(moveType, next, pawnAt, m.pawnStop)) {
			end = true
			break
		}
	}
	turn := m.turnLocked(stepFrom)
	if end {
		return chain(turn, m.endLocked())
	}
	if player {
		m.lastUpdate = m.nowLocked()
	}
	m.retimeLocked()
	return turn
}

// turnLocked is the turn a tracking pawn walk's step from from to the
// origin takes, for the caller to run after unlocking; nil for any other
// walk. Callers hold mu.
func (m *CreatureMove) turnLocked(from location.Location) func() {
	owner := m.owner
	if owner == nil || !m.pawnTracks {
		return nil
	}
	to := m.origin
	return func() { owner.pawnStepped(from, to) }
}

// chain runs first and then second, either of which may be nil.
func chain(first, second func()) func() {
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	}
	return func() {
		first()
		second()
	}
}

// endLocked stops the move where the actor stands and returns the arrival
// hook for the caller to invoke after unlocking. Callers hold mu.
func (m *CreatureMove) endLocked() func() {
	m.rescheduleLocked(0)
	m.waypoints = nil
	m.moving = false
	m.pawn = nil
	return m.arrivalHookLocked()
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
	m.setAccurateLocked(m.destination)

	// Advance through any remaining waypoints. Zero-distance segments are
	// skipped silently; a positive-distance segment becomes the active
	// destination with a freshly scheduled arrival timer. Scheduling a new
	// timer advances moveSeq, so the previous segment's onArrive callback
	// (if still in flight) is ignored.
	for len(m.waypoints) > 0 {
		next := m.waypoints[0]
		m.waypoints = m.waypoints[1:]
		distance := between(m.moveTypeLocked(), m.origin, next)
		ticks := m.travelTicksLocked(distance)
		if math.IsNaN(ticks) || ticks > maxTravelTicks {
			// Unrepresentable next-segment duration: stop here, drop tail.
			return m.endLocked()
		}
		if ticks <= 0 {
			// Zero-distance segment: snap forward and continue.
			m.destination = next
			m.origin = next
			m.setAccurateLocked(next)
			continue
		}
		m.destination = next
		m.moving = true
		m.rescheduleLocked(m.arrivalDelayLocked())
		ev := m.currentEventLocked()
		owner := m.owner
		if owner == nil {
			return nil
		}
		return func() { owner.segmentAdvanced(ev) }
	}

	return m.endLocked()
}

// UpdatePosition advances one in-flight movement by step and reports the
// current movement event to broadcast. It returns false after the move has
// already stopped or reaches its final destination. Reaching an intermediate
// waypoint — including when the current leg is blocked and a later waypoint
// remains — returns the next segment's event with true.
//
// On the last leg of a pawn walk toward a creature the walk ends at the
// first position within the walk's offset of the pawn's current position (2D
// on the ground, 3D swimming or flying); a walk toward any other pawn runs
// onto its point. A tracking pawn walk first re-aims the leg at that
// position.
//
// The update stands for step of walking, for a player too: the production
// tick (Controller.PositionUpdate) measures a player's step on the queue
// clock instead.
func (m *CreatureMove) UpdatePosition(step time.Duration) (event.Move, bool) {
	u := m.updatePosition(step, false)
	if u.hook != nil {
		u.hook()
	}
	return u.ev, u.moving
}

// positionUpdate is one position update's outcome. hook is the milestone to
// run once the caller has applied the step; pawnStep reports a step of a
// tracking pawn walk, from the cell from to the cell to, the walker turns
// along.
type positionUpdate struct {
	ev       event.Move
	moving   bool
	hook     func()
	pawnStep bool
	from, to location.Location
}

// updatePosition runs one position update standing for step. With measured,
// a player's update instead stands for the queue-clock time since its last
// update, the move start or a retarget catch-up, less than a millisecond
// counting as one; any other mover's still stands for step.
func (m *CreatureMove) updatePosition(step time.Duration, measured bool) positionUpdate {
	// Read the pawn's position before taking mu: it may take the pawn's
	// own locks.
	m.mu.Lock()
	pawn, pawnGen := m.pawn, m.pawnGen
	m.mu.Unlock()
	var pawnAt location.Location
	if pawn != nil {
		x, y, z := pawn.Position()
		pawnAt = location.Location{X: x, Y: y, Z: z}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.moving {
		return positionUpdate{}
	}
	player := m.playerStepsLocked()
	var now time.Time
	if player {
		now = m.nowLocked()
		if measured {
			step = now.Sub(m.lastUpdate)
		}
	}
	if step <= 0 && !(player && measured) {
		return positionUpdate{ev: m.currentEventLocked(), moving: true}
	}
	m.updates++

	lastPawnLeg := pawn != nil && pawnGen == m.pawnGen && m.onLastPawnLegLocked()
	tracking := lastPawnLeg && m.pawnTracks
	if tracking {
		m.destination = pawnAt
	}
	var passed float64
	if player {
		passed = playerPassed(m.updateSpeedLocked(), step)
		m.lastUpdate = now
	} else {
		passed = m.updateSpeedLocked() * step.Seconds()
	}
	moveType, maxZ := m.waterLocked()
	next, nextAccurate, reached := m.stepLocked(passed, moveType, maxZ)
	if moveType == MoveGround && !m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, next.X, next.Y, next.Z) {
		m.routeBlocked = true
		ok, hook := m.startNextWaypointLocked()
		if !ok {
			return positionUpdate{hook: m.stopBlockedLocked()}
		}
		return positionUpdate{ev: m.currentEventLocked(), moving: true, hook: hook}
	}
	u := positionUpdate{pawnStep: tracking, from: m.origin, to: next}
	if reached {
		u.hook = m.finishLocked()
		if !m.moving {
			return u
		}
		// Advanced to the next waypoint segment; report it and fire the
		// segment-advanced hook (if any) so the client is told about the new
		// leg the same tick the server itself commits to it.
		u.ev, u.moving = m.currentEventLocked(), true
		return u
	}

	m.accurateX, m.accurateY, m.accurateZ = nextAccurate[0], nextAccurate[1], nextAccurate[2]
	m.origin = next
	if lastPawnLeg && m.pawnStop > 0 && inRadius(moveType, next, pawnAt, m.pawnStop) {
		u.hook = m.endLocked()
		return u
	}
	// The re-aimed leg's arrival moves with the pawn, and a leg whose
	// mover changed move type (a wade from the shore into water) now
	// measures what is left in the other way.
	if tracking || m.moveTypeLocked() != m.timedType {
		m.retimeLocked()
	}
	u.ev, u.moving = m.currentEventLocked(), true
	return u
}

// stepLocked is where a mover of type moveType advancing passed units
// toward the destination lands, with the accurate position (x, y, z) there;
// reached reports that the step covers the rest of the leg, landing on the
// destination itself. A ground mover snaps the destination to the floor
// capped at maxZ first, and lands on the floor; a swimming or flying one
// measures the leg in 3D and moves its height toward the destination's,
// capped at maxZ. A player measures the step from its current cell and
// rounds the cell it lands on; any other mover measures it from its
// accurate position and truncates. Callers hold mu.
func (m *CreatureMove) stepLocked(passed float64, moveType MoveType, maxZ int) (next location.Location, accurate [3]float64, reached bool) {
	if moveType == MoveGround {
		m.destination.Z = min(int(m.geo.Height(m.destination.X, m.destination.Y, m.destination.Z)), maxZ)
	}
	fromX, fromY := m.accurateX, m.accurateY
	player := m.playerStepsLocked()
	if player {
		fromX, fromY = float64(m.origin.X), float64(m.origin.Y)
	}
	dx := float64(m.destination.X) - fromX
	dy := float64(m.destination.Y) - fromY
	dz := float64(m.destination.Z) - float64(m.origin.Z)
	left := span(moveType, dx, dy, dz)
	if left == 0 || passed >= left {
		next = m.destination
		next.Z = min(next.Z, maxZ)
		return next, [3]float64{m.accurateX, m.accurateY, m.accurateZ}, true
	}
	fraction := passed / left
	accurate = [3]float64{m.accurateX + dx*fraction, m.accurateY + dy*fraction, m.accurateZ + dz*fraction}
	if player {
		next.X, next.Y = roundHalfUp(accurate[0]), roundHalfUp(accurate[1])
	} else {
		next.X, next.Y = int(accurate[0]), int(accurate[1])
	}
	// A player carries its accurate height from one step to the next under
	// every move type, though only a swimming or flying one lands on it: one
	// that wades in mid-leg swims on from the height its ground steps added
	// up. Any other mover steps from the cell it stands on.
	switch {
	case moveType == MoveGround:
		next.Z = min(int(m.geo.Height(next.X, next.Y, m.origin.Z+2*block.CellHeight)), maxZ)
		if !player {
			accurate[2] = float64(next.Z)
		}
	case player:
		next.Z = min(roundHalfUp(accurate[2]), maxZ)
	default:
		next.Z = min(m.origin.Z+int(dz*fraction+0.5), maxZ)
		accurate[2] = float64(next.Z)
	}
	return next, accurate, false
}

// catchUpLocked is the update a player's walk in flight runs before a
// retarget: it advances toward the current destination, without re-aiming a
// pawn walk, by the time passed since the last update, and counts as a
// position update. A closed line leaves the walker in place and the walk
// blocked. Any other catch-up is a step, counted in catchUps even when it
// stays on the same cell. It runs no milestone, even on reaching the
// destination. Callers hold mu.
func (m *CreatureMove) catchUpLocked() {
	elapsed := m.nowLocked().Sub(m.lastUpdate)
	m.updates++
	moveType, maxZ := m.waterLocked()
	next, accurate, _ := m.stepLocked(playerPassed(m.updateSpeedLocked(), elapsed), moveType, maxZ)
	if moveType == MoveGround && !m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, next.X, next.Y, next.Z) {
		m.routeBlocked = true
		return
	}
	m.accurateX, m.accurateY, m.accurateZ = accurate[0], accurate[1], accurate[2]
	m.origin = next
	m.catchUps++
}

// catchUpCount returns how many retarget catch-ups stepped the mover so far.
func (m *CreatureMove) catchUpCount() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.catchUps
}

// abandonPawnWalk is the position update of the pawn walk gen once its pawn
// is no longer known: the actor does not step, and the walk heads for its
// next queued geopath leg from where the actor stands or, with none left,
// ends there as an arrival. It reports false, with no update, when a later
// request (another gen) replaced the walk.
func (m *CreatureMove) abandonPawnWalk(gen uint64) (positionUpdate, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.moving || m.pawn == nil || m.pawnGen != gen {
		return positionUpdate{}, false
	}
	if ok, hook := m.startNextWaypointLocked(); ok {
		return positionUpdate{ev: m.currentEventLocked(), moving: true, hook: hook}, true
	}
	return positionUpdate{hook: m.endLocked()}, true
}

// walkingPawn returns the pawn of the pawn walk under way and its gen, or
// nil.
func (m *CreatureMove) walkingPawn() (Pawn, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.moving {
		return nil, 0
	}
	return m.pawn, m.pawnGen
}

// maxZLocked is the highest a step from the origin may land. Callers hold
// mu.
func (m *CreatureMove) maxZLocked() int {
	_, maxZ := m.waterLocked()
	return maxZ
}

// waterLocked is the mover's MoveType at its origin (moveTypeLocked) and the
// highest a step from there may land: the surface of the water zone the
// mover stands in when the floor lies deeper than waterDepthMin below it,
// else the world max. It queries the water zone once. Callers hold mu.
//
// A UseZoneSwim mover (a player) is held at the surface only while it
// flies, never while it swims: a flyer whose zones do not hold it in water
// yet stays capped at the surface of the water zone it stands in until they
// do.
func (m *CreatureMove) waterLocked() (MoveType, int) {
	if m.zoneSwim {
		moveType := m.flagTypeLocked()
		if moveType == MoveFly {
			if surface, ok := m.surfaceCapLocked(); ok {
				return moveType, surface
			}
		}
		return moveType, worldZMax
	}
	if m.water != nil {
		if surface, ok := m.water(m.origin); ok {
			if !m.playerStepsLocked() && int(m.geo.Height(m.origin.X, m.origin.Y, m.origin.Z))-surface < -waterDepthMin {
				return MoveSwim, surface
			}
			return MoveSwim, worldZMax
		}
	}
	if m.flying {
		return MoveFly, worldZMax
	}
	return MoveGround, worldZMax
}

// surfaceCapLocked is the surface of the water zone at the origin when the
// floor there lies deeper than waterDepthMin below it. Callers hold mu.
func (m *CreatureMove) surfaceCapLocked() (int, bool) {
	if m.water == nil {
		return 0, false
	}
	surface, ok := m.water(m.origin)
	if !ok || int(m.geo.Height(m.origin.X, m.origin.Y, m.origin.Z))-surface >= -waterDepthMin {
		return 0, false
	}
	return surface, true
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
	m.pawn = nil
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
	m.setAccurateLocked(m.origin)
	m.destination = next

	distance := between(m.moveTypeLocked(), m.origin, next)
	ticks := m.travelTicksLocked(distance)
	if math.IsNaN(ticks) || ticks > maxTravelTicks {
		m.waypoints = nil
		m.moving = false
		m.pawn = nil
		return false, nil
	}
	m.moving = true
	m.rescheduleLocked(m.arrivalDelayLocked())
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
	m.pawn = nil
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

// MovingTo returns the target of the leg in flight — the first geopath
// segment of a routed walk — and whether a walk is in flight at all.
func (m *CreatureMove) MovingTo() (location.Location, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.destination, m.moving
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

// FollowTarget returns the object id the active follow task follows, or 0.
func (m *CreatureMove) FollowTarget() int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.followTarget
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

	if inRadius(m.moveTypeLocked(), m.origin, target.Position, followRange(m.followOffset, actorRadius, target.CollisionRadius)) {
		return event.Move{}, false, nil
	}

	followMode := m.followMode
	followOffset := m.followOffset
	ev, _, err := m.moveToLocationLocked(target.Position, nil, 0, false, true)
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
