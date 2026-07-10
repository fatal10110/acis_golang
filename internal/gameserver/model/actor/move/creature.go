// Package move models a creature's requested movement state.
package move

import (
	"errors"
	"math"
	"reflect"
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
}

// CreatureMove holds movement state owned and updated by one caller.
type CreatureMove struct {
	origin, destination location.Location
	speed               float64
	geo                 Geo
	moving              bool
}

// NewCreatureMove builds movement state at origin with a positive ground speed.
func NewCreatureMove(origin location.Location, speed float64, geo Geo) (*CreatureMove, error) {
	if geo == nil || (reflect.ValueOf(geo).Kind() == reflect.Ptr && reflect.ValueOf(geo).IsNil()) {
		return nil, errors.New("move: nil geodata")
	}
	if speed <= 0 {
		return nil, errors.New("move: speed must be positive")
	}
	return &CreatureMove{origin: origin, destination: origin, speed: speed, geo: geo}, nil
}

// MoveToLocation records an accepted, height-normalized ground-movement request.
func (m *CreatureMove) MoveToLocation(target location.Location) (Event, error) {
	target.Z = int(m.geo.Height(target.X, target.Y, target.Z))
	if !m.geo.CanMove(m.origin.X, m.origin.Y, m.origin.Z, target.X, target.Y, target.Z) {
		return Event{}, errors.New("move: route is blocked")
	}

	distance := math.Hypot(float64(target.X-m.origin.X), float64(target.Y-m.origin.Y))
	duration := time.Duration(math.Ceil(distance/(m.speed/10))) * 100 * time.Millisecond
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
