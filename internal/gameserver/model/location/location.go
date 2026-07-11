// Package location contains world-coordinate datatypes.
package location

import "github.com/fatal10110/acis_golang/internal/commons"

// Location is a 3D (x/y/z) world point.
type Location struct {
	X, Y, Z int
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
