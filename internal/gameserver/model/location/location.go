// Package location contains world-coordinate datatypes.
package location

import "github.com/fatal10110/acis_golang/internal/commons"

// Location is a 3D (x/y/z) world point.
type Location struct {
	X, Y, Z int
}

// NewLocation builds a Location from set; x, y and z are all required.
func NewLocation(set *commons.StatSet) (Location, error) {
	x, err := set.GetInt("x")
	if err != nil {
		return Location{}, err
	}
	y, err := set.GetInt("y")
	if err != nil {
		return Location{}, err
	}
	z, err := set.GetInt("z")
	if err != nil {
		return Location{}, err
	}
	return Location{X: x, Y: y, Z: z}, nil
}
