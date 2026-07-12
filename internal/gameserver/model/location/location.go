// Package location contains world-coordinate datatypes.
package location

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Location is a 3D (x/y/z) world point.
type Location struct {
	X, Y, Z int
}

// Distance2D returns the flat ground distance between l and other, ignoring
// the Z axis.
func (l Location) Distance2D(other Location) float64 {
	dx := float64(l.X - other.X)
	dy := float64(l.Y - other.Y)
	return math.Hypot(dx, dy)
}

// headingScale converts a full-circle angle in degrees to the game's
// heading range (65536 units per circle): 65536 / 360.
const headingScale = 182.04444444444444

// HeadingTo returns the game heading (0..65535) that faces directly from l
// toward other, ignoring the Z axis. l and other equal is a zero heading.
func (l Location) HeadingTo(other Location) int {
	angle := math.Atan2(float64(other.Y-l.Y), float64(other.X-l.X)) * 180 / math.Pi
	if angle < 0 {
		angle += 360
	}
	return int(math.Round(angle * headingScale))
}

// NewLocation builds a Location from set; x, y and z are all required.
func NewLocation(set *commons.StatSet) (Location, error) {
	f := commons.NewFields(set, "location")
	loc := Location{
		X: f.Int("x"),
		Y: f.Int("y"),
		Z: f.Int("z"),
	}
	if err := f.Err(); err != nil {
		return Location{}, err
	}
	return loc, nil
}
