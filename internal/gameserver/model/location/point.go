package location

import "github.com/fatal10110/acis_golang/internal/commons"

// Point is a 2D (x/y) world coordinate.
type Point struct {
	X, Y int
}

// NewPoint builds a Point from set; x and y are required.
func NewPoint(set *commons.StatSet) (Point, error) {
	x, err := set.GetInt("x")
	if err != nil {
		return Point{}, err
	}
	y, err := set.GetInt("y")
	if err != nil {
		return Point{}, err
	}
	return Point{X: x, Y: y}, nil
}
