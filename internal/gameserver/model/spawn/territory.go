package spawn

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
)

// Node is one 2D polygon point from a <territory><node .../></territory>.
type Node struct {
	X int
	Y int
}

// Territory is one named spawn polygon with the vertical range the datapack
// declares for it. Name/MinZ/MaxZ/Nodes stay directly readable for existing
// point-in-territory placement logic; the embedded *geometry.Territory
// (nil unless built through NewTerritory) adds Area and Intersects, plus a
// Contains(x, y, z) that agrees with MinZ/MaxZ/Nodes by construction.
type Territory struct {
	Name  string
	MinZ  int
	MaxZ  int
	Nodes []Node
	*geometry.Territory
}

// geometryTerritory returns the prebuilt shape, or triangulates the nodes on
// demand for a Territory assembled as a struct literal.
//
// ponytail: the on-demand path re-triangulates per call. Only test fixtures
// take it — everything loaded from the datapack goes through NewTerritory and
// has the pointer set — so cache it here only if a production path ever
// constructs a Territory without it.
func (t *Territory) geometryTerritory() *geometry.Territory {
	if t == nil {
		return nil
	}
	if t.Territory != nil {
		return t.Territory
	}
	points := make([]geometry.Point, len(t.Nodes))
	for i, n := range t.Nodes {
		points[i] = geometry.Point{X: n.X, Y: n.Y}
	}
	poly, err := geometry.NewTriangulatedPolygon(points)
	if err != nil {
		return nil
	}
	shape, err := geometry.NewTerritory(t.MinZ, t.MaxZ, poly)
	if err != nil {
		return nil
	}
	return shape
}

// Contains reports whether (x, y, z) lies inside the territory.
func (t *Territory) Contains(x, y, z int) bool {
	shape := t.geometryTerritory()
	return shape != nil && shape.Contains(x, y, z)
}

// Contains2D reports whether (x, y) lies inside the territory footprint,
// ignoring z.
func (t *Territory) Contains2D(x, y int) bool {
	shape := t.geometryTerritory()
	return shape != nil && shape.Contains2D(x, y)
}

// Area reports the territory's 2D area.
func (t *Territory) Area() float64 {
	shape := t.geometryTerritory()
	if shape == nil {
		return 0
	}
	return shape.Area()
}

// Intersects reports whether this territory overlaps other.
func (t *Territory) Intersects(other *geometry.Territory) bool {
	shape := t.geometryTerritory()
	return shape != nil && other != nil && shape.Intersects(other)
}

// ErrTerritoryBuild marks a territory whose polygon could not be built (too
// few nodes, an inverted Z range, or a shape triangulation rejects). It
// mirrors the boundary of SpawnManager.java's per-territory try/catch around
// `new Territory(name, Kong.doTriangulation(coords), minZ, maxZ)`: the name
// and minZ/maxZ attribute reads happen before that try and still propagate,
// but everything from there on is caught, warned about, and skipped by the
// caller instead of aborting the whole spawnlist load.
var ErrTerritoryBuild = errors.New("spawn: territory build failed")

// NewTerritory builds a Territory from set plus its decoded polygon nodes.
func NewTerritory(set *commons.StatSet, nodes []Node) (*Territory, error) {
	idf := commons.NewFields(set, "spawn territory")
	name := idf.String("name")
	if err := idf.Err(); err != nil {
		return nil, err
	}
	f := commons.NewFields(set, fmt.Sprintf("spawn territory %q", name))
	minZ := f.Int("minZ")
	maxZ := f.Int("maxZ")
	if err := f.Err(); err != nil {
		return nil, err
	}
	if len(nodes) < 3 {
		return nil, fmt.Errorf("%w: territory %q needs at least 3 nodes", ErrTerritoryBuild, name)
	}
	if maxZ < minZ {
		return nil, fmt.Errorf("%w: territory %q maxZ must be >= minZ", ErrTerritoryBuild, name)
	}

	copyNodes := append([]Node(nil), nodes...)
	points := make([]geometry.Point, len(nodes))
	for i, n := range nodes {
		points[i] = geometry.Point{X: n.X, Y: n.Y}
	}
	poly, err := geometry.NewTriangulatedPolygon(points)
	if err != nil {
		return nil, fmt.Errorf("%w: territory %q: %v", ErrTerritoryBuild, name, err)
	}
	shape, err := geometry.NewTerritory(minZ, maxZ, poly)
	if err != nil {
		return nil, fmt.Errorf("%w: territory %q: %v", ErrTerritoryBuild, name, err)
	}

	return &Territory{
		Name:      name,
		MinZ:      minZ,
		MaxZ:      maxZ,
		Nodes:     copyNodes,
		Territory: shape,
	}, nil
}
