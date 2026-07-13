// Package move models a creature's requested movement state.
package move

import (
	"errors"
	"math"
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
type CreatureMove struct {
	origin, destination location.Location
	speed               float64
	geo                 Geo
	moving              bool
	followTarget        int32
	followOffset        int
	followMode          FollowMode
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

// MoveToLocation records an accepted, height-normalized ground-movement request.
func (m *CreatureMove) MoveToLocation(target location.Location) (Event, error) {
	target.Z = int(m.geo.Height(target.X, target.Y, target.Z))
	if !m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z) {
		return Event{}, errors.New("move: route is blocked")
	}
	if target.X == m.origin.X && target.Y == m.origin.Y {
		m.destination = target
		m.moving = false
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
	m.destination = target
	m.moving = duration > 0

	return Event{
		Origin:      m.origin,
		Destination: target,
		Speed:       m.speed,
		Duration:    duration,
	}, nil
}

// Moving reports whether the current request has non-zero ground distance.
func (m *CreatureMove) Moving() bool {
	return m.moving
}

// Destination returns the target of the last accepted movement request.
func (m *CreatureMove) Destination() location.Location {
	return m.destination
}

// StartFriendlyFollow starts a friendly follow task for targetID.
func (m *CreatureMove) StartFriendlyFollow(targetID int32, offset int) {
	m.followMode = FollowFriendly
	m.followTarget = targetID
	m.followOffset = offset
}

// StartOffensiveFollow starts an offensive follow task for targetID.
func (m *CreatureMove) StartOffensiveFollow(targetID int32, offset int) {
	m.followMode = FollowOffensive
	m.followTarget = targetID
	m.followOffset = offset
}

// CancelFollow clears any active follow task.
func (m *CreatureMove) CancelFollow() {
	m.followMode = FollowNone
	m.followTarget = 0
	m.followOffset = 0
}

// Following reports whether a follow task is active.
func (m *CreatureMove) Following() bool {
	return m.followMode != FollowNone
}

// FollowMode returns the active follow mode.
func (m *CreatureMove) FollowMode() FollowMode {
	return m.followMode
}

// FollowInterval returns how often the active follow task should be ticked.
func (m *CreatureMove) FollowInterval() time.Duration {
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
	if m.followMode == FollowNone || target.ObjectID != m.followTarget || !target.Known {
		return Event{}, false, nil
	}
	if m.followMode == FollowFriendly && target.InBoat {
		return Event{}, false, nil
	}

	if in2DRange(m.origin, target.Position, followRange(m.followOffset, actorRadius, target.CollisionRadius)) {
		return Event{}, false, nil
	}

	event, err := m.MoveToLocation(target.Position)
	if err != nil {
		return Event{}, false, err
	}
	if m.followMode == FollowOffensive {
		event.FollowTarget = target.ObjectID
		event.FollowOffset = m.followOffset
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
