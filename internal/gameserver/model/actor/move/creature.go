// Package move models a creature's requested movement state.
package move

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Event describes one accepted movement request.
type Event struct {
	Origin, Destination location.Location
	Speed               float64
	Duration            time.Duration
	FollowTarget        int32
	FollowOffset        int
}

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
// every mutable field below, since an accepted MoveToLocation schedules a
// timer goroutine that advances origin and fires the arrived hook
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
// movement correction cadence and may complete the move first.
type CreatureMove struct {
	geo Geo

	mu                   sync.Mutex
	origin, destination  location.Location
	waypoints            []location.Location
	accurateX, accurateY float64
	speed                float64
	moving               bool
	followTarget         int32
	followOffset         int
	followMode           FollowMode
	arrived              func()
	timer                scheduledTimer
	moveSeq              uint64
	afterFunc            func(time.Duration, func()) scheduledTimer
}

type scheduledTimer interface {
	Stop() bool
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

// SetArrivedHook records the callback fired once an accepted move reaches
// its destination. A nil hook (the default) makes arrival a no-op.
func (m *CreatureMove) SetArrivedHook(arrived func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.arrived = arrived
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

// MoveToLocation records an accepted, height-normalized ground-movement
// request and, once its Duration elapses, advances the actor's position to
// destination and fires the arrived hook.
func (m *CreatureMove) MoveToLocation(target location.Location) (Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveToLocationLocked(target)
}

func (m *CreatureMove) moveToLocationLocked(target location.Location) (Event, error) {
	target.Z = int(m.geo.Height(target.X, target.Y, target.Z))

	// Same-cell requests resolve as a zero-distance completion: no
	// pathfinding query, no arrival timer.
	if target.X == m.origin.X && target.Y == m.origin.Y {
		m.waypoints = nil
		m.destination = target
		m.accurateX = float64(m.origin.X)
		m.accurateY = float64(m.origin.Y)
		m.moving = false
		m.rescheduleLocked(0)
		return Event{Origin: m.origin, Destination: target, Speed: m.speed}, nil
	}

	if m.speed == 0 {
		return Event{}, errors.New("move: actor cannot move at zero speed")
	}

	destination, waypoints, err := m.resolvePathLocked(target)
	if err != nil {
		return Event{}, err
	}

	distance := math.Hypot(float64(destination.X)-float64(m.origin.X), float64(destination.Y)-float64(m.origin.Y))
	ticks := math.Ceil(distance / (m.speed / 10))
	const tickDuration = 100 * time.Millisecond
	if math.IsNaN(ticks) || ticks > float64(time.Duration(1<<63-1)/tickDuration) {
		return Event{}, errors.New("move: duration exceeds limit")
	}
	duration := time.Duration(ticks) * tickDuration
	origin := m.origin
	m.accurateX = float64(origin.X)
	m.accurateY = float64(origin.Y)
	m.destination = destination
	m.waypoints = waypoints
	m.moving = duration > 0
	m.rescheduleLocked(duration)

	return Event{
		Origin:      origin,
		Destination: destination,
		Speed:       m.speed,
		Duration:    duration,
	}, nil
}

// resolvePathLocked applies the three-tier route resolution a move request
// uses: a straight-line reachability check (tier 1), a routed search around
// the obstacle (tier 2), and a partial-progress last reachable point when
// the route cannot complete (tier 3). The returned destination is the
// first segment to walk; waypoints holds the remaining segments, if any.
//
// origin and target carry geodata-snapped Z.
func (m *CreatureMove) resolvePathLocked(target location.Location) (location.Location, []location.Location, error) {
	if m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z) {
		return target, nil, nil
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
		return destination, tail, nil
	}

	fallback := m.geo.ValidLocation(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z)
	// A fall-back that resolves to the origin itself means walking the line
	// makes no lateral progress: the route is genuinely blocked, so the
	// request fails without disturbing the prior destination.
	if fallback.X == m.origin.X && fallback.Y == m.origin.Y {
		return location.Location{}, nil, errors.New("move: route is blocked")
	}
	return fallback, nil, nil
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
	source := m.afterFunc
	if source == nil {
		source = func(d time.Duration, f func()) scheduledTimer {
			return time.AfterFunc(d, f)
		}
	}
	m.timer = source(duration, func() { m.onArrive(seq) })
}

func (m *CreatureMove) onArrive(seq uint64) {
	m.mu.Lock()
	if seq != m.moveSeq || !m.moving {
		m.mu.Unlock()
		return
	}
	arrived := m.finishLocked()
	m.mu.Unlock()

	if arrived != nil {
		arrived()
	}
}

func (m *CreatureMove) finishLocked() func() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	// Snap to the just-completed segment's destination.
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
			return m.arrived
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
		return nil
	}

	m.moving = false
	return m.arrived
}

// UpdatePosition advances one in-flight movement by step and reports the
// current movement event to broadcast. It returns false after the move has
// already stopped or reaches its final destination; reaching an intermediate
// waypoint segment returns the next segment's event with true.
func (m *CreatureMove) UpdatePosition(step time.Duration) (Event, bool) {
	m.mu.Lock()
	if !m.moving {
		m.mu.Unlock()
		return Event{}, false
	}
	if step <= 0 {
		event := m.currentEventLocked()
		m.mu.Unlock()
		return event, true
	}

	dx := float64(m.destination.X) - m.accurateX
	dy := float64(m.destination.Y) - m.accurateY
	left := math.Hypot(dx, dy)
	passed := m.speed * step.Seconds()
	if left == 0 || passed >= left {
		arrived := m.finishLocked()
		if arrived != nil {
			m.mu.Unlock()
			arrived()
			return Event{}, false
		}
		// Advanced to the next waypoint segment; report it.
		event := m.currentEventLocked()
		m.mu.Unlock()
		return event, m.moving
	}

	fraction := passed / left
	m.accurateX += dx * fraction
	m.accurateY += dy * fraction
	nextX := int(m.accurateX)
	nextY := int(m.accurateY)
	m.origin = location.Location{
		X: nextX,
		Y: nextY,
		Z: int(m.geo.Height(nextX, nextY, m.origin.Z)),
	}
	event := m.currentEventLocked()
	m.mu.Unlock()
	return event, true
}

func (m *CreatureMove) currentEventLocked() Event {
	event := Event{
		Origin:      m.origin,
		Destination: m.destination,
		Speed:       m.speed,
	}
	if m.followMode == FollowOffensive {
		event.FollowTarget = m.followTarget
		event.FollowOffset = m.followOffset
	}
	return event
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
func (m *CreatureMove) FollowTick(target TargetSnapshot, actorRadius float64) (Event, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.followMode == FollowNone || target.ObjectID != m.followTarget || !target.Known {
		return Event{}, false, nil
	}
	if m.followMode == FollowFriendly && target.InBoat {
		return Event{}, false, nil
	}

	if in2DRange(m.origin, target.Position, followRange(m.followOffset, actorRadius, target.CollisionRadius)) {
		return Event{}, false, nil
	}

	followMode := m.followMode
	followOffset := m.followOffset
	event, err := m.moveToLocationLocked(target.Position)
	if err != nil {
		return Event{}, false, err
	}
	if followMode == FollowOffensive {
		event.FollowTarget = target.ObjectID
		event.FollowOffset = followOffset
	}
	return event, true, nil
}

func followRange(offset int, actorRadius, targetRadius float64) int {
	return int(float64(offset) + actorRadius + targetRadius)
}

func in2DRange(origin, target location.Location, radius int) bool {
	if radius < 0 {
		return false
	}
	return math.Hypot(float64(target.X)-float64(origin.X), float64(target.Y)-float64(origin.Y)) <= float64(radius)
}
