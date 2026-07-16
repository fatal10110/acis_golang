// Package move models a creature's requested movement state.
package move

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Geo supplies the terrain queries required to validate ground movement.
type Geo interface {
	CanMove(ox, oy, oz, tx, ty, tz int) bool
	Height(x, y, z int) int16
}

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
// A straight-line move snaps origin to destination once its Duration
// elapses, rather than interpolating intermediate positions every 100ms the
// way the reference client-correction ticker does — a deliberate
// simplification. Nothing in this port depends on sub-tick position
// accuracy mid-move; only the end state (arrived, or still moving) matters
// for range checks and AI re-evaluation.
type CreatureMove struct {
	geo Geo

	mu                  sync.Mutex
	origin, destination location.Location
	speed               float64
	moving              bool
	followTarget        int32
	followOffset        int
	followMode          FollowMode
	arrived             func()
	timer               scheduledTimer
	moveSeq             uint64
	afterFunc           func(time.Duration, func()) scheduledTimer
}

type scheduledTimer interface {
	Stop() bool
}

// NewCreatureMove builds movement state at origin with a non-negative ground
// speed. Zero is a valid, stationary speed (e.g. an immobile scripted NPC) —
// MoveToLocation rejects any actual movement request once speed is zero.
func NewCreatureMove(origin location.Location, speed float64, geo Geo) (*CreatureMove, error) {
	if geo == nil {
		return nil, errors.New("move: nil geodata")
	}
	if speed < 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		return nil, errors.New("move: speed must not be negative")
	}
	return &CreatureMove{origin: origin, destination: origin, speed: speed, geo: geo}, nil
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
	if m.destination == position {
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
	if !m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z) {
		return Event{}, errors.New("move: route is blocked")
	}
	if target.X == m.origin.X && target.Y == m.origin.Y {
		m.destination = target
		m.moving = false
		m.rescheduleLocked(0)
		return Event{Origin: m.origin, Destination: target, Speed: m.speed}, nil
	}

	if m.speed == 0 {
		return Event{}, errors.New("move: actor cannot move at zero speed")
	}

	distance := math.Hypot(float64(target.X)-float64(m.origin.X), float64(target.Y)-float64(m.origin.Y))
	ticks := math.Ceil(distance / (m.speed / 10))
	const tickDuration = 100 * time.Millisecond
	if math.IsNaN(ticks) || ticks > float64(time.Duration(1<<63-1)/tickDuration) {
		return Event{}, errors.New("move: duration exceeds limit")
	}
	duration := time.Duration(ticks) * tickDuration
	origin := m.origin
	m.destination = target
	m.moving = duration > 0
	m.rescheduleLocked(duration)

	return Event{
		Origin:      origin,
		Destination: target,
		Speed:       m.speed,
		Duration:    duration,
	}, nil
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
	m.origin = m.destination
	m.moving = false
	m.timer = nil
	arrived := m.arrived
	m.mu.Unlock()

	if arrived != nil {
		arrived()
	}
}

// CancelMove stops any pending arrival timer and leaves the actor at its
// current position, without changing follow state.
func (m *CreatureMove) CancelMove() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rescheduleLocked(0)
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
