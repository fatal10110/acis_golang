package spawn

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Node is one 2D polygon point from a <territory><node .../></territory>.
type Node struct {
	X int
	Y int
}

// Territory is one named spawn polygon with the vertical range the datapack
// declares for it.
type Territory struct {
	Name  string
	MinZ  int
	MaxZ  int
	Nodes []Node
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
	return &Territory{
		Name:  name,
		MinZ:  minZ,
		MaxZ:  maxZ,
		Nodes: copyNodes,
	}, nil
}
