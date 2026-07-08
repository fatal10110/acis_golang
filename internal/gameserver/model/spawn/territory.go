package spawn

import (
	"errors"

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
	name, err := set.GetString("name")
	if err != nil {
		return nil, err
	}
	minZ, err := set.GetInt("minZ")
	if err != nil {
		return nil, err
	}
	maxZ, err := set.GetInt("maxZ")
	if err != nil {
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
