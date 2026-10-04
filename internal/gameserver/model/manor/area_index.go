package manor

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
)

// AreaIndex answers which manor area holds a point. An area is the
// triangulated footprint of its nodes between its MinZ and MaxZ, both
// bounds inclusive.
type AreaIndex struct {
	areas []indexedArea
}

type indexedArea struct {
	area      Area
	territory *geometry.Territory
}

// NewAreaIndex indexes areas in order. An area whose nodes do not
// triangulate is left out and reported in skipped; the others still load.
func NewAreaIndex(areas AreaTable) (index *AreaIndex, skipped []error) {
	index = &AreaIndex{areas: make([]indexedArea, 0, len(areas))}
	for _, a := range areas {
		points := make([]geometry.Point, len(a.Nodes))
		for i, n := range a.Nodes {
			points[i] = geometry.Point{X: n.X, Y: n.Y}
		}
		poly, err := geometry.NewTriangulatedPolygon(points)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("manor area %q: %w", a.Name, err))
			continue
		}
		territory, err := geometry.NewTerritory(a.MinZ, a.MaxZ, poly)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("manor area %q: %w", a.Name, err))
			continue
		}
		index.areas = append(index.areas, indexedArea{area: a, territory: territory})
	}
	return index, skipped
}

// Len is the number of indexed areas.
func (x *AreaIndex) Len() int {
	if x == nil {
		return 0
	}
	return len(x.areas)
}

// AreaAt returns the first area, in index order, holding (px, py, pz).
func (x *AreaIndex) AreaAt(px, py, pz int) (Area, bool) {
	if x == nil {
		return Area{}, false
	}
	for _, a := range x.areas {
		if a.territory.Contains(px, py, pz) {
			return a.area, true
		}
	}
	return Area{}, false
}

// Seed returns the seed row whose seed item is seedID.
func (t *Table) Seed(seedID int32) (Seed, bool) {
	if t == nil {
		return Seed{}, false
	}
	seed, ok := t.SeedsByID[int(seedID)]
	return seed, ok
}
