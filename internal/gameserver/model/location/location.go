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
