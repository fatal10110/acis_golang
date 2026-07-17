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
		return nil, errors.New("spawn: territory needs at least 3 nodes")
	}
	if maxZ < minZ {
		return nil, errors.New("spawn: territory maxZ must be >= minZ")
	}

	copyNodes := append([]Node(nil), nodes...)
	points := make([]geometry.Point, len(nodes))
	for i, n := range nodes {
		points[i] = geometry.Point{X: n.X, Y: n.Y}
	}
	poly, err := geometry.NewPolygon(points)
	if err != nil {
		return nil, fmt.Errorf("spawn: territory %q: %w", name, err)
	}
	shape, err := geometry.NewTerritory(minZ, maxZ, poly)
	if err != nil {
		return nil, fmt.Errorf("spawn: territory %q: %w", name, err)
	}

	return &Territory{
		Name:      name,
		MinZ:      minZ,
		MaxZ:      maxZ,
		Nodes:     copyNodes,
		Territory: shape,
	}, nil
}
