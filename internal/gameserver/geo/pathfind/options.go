package pathfind

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/config"
)

const (
	defaultMoveWeight      = 10
	defaultMoveWeightDiag  = 14
	defaultObstacleWeight  = 30
	defaultHeuristicWeight = 12
	defaultMaxIterations   = 10000
)

// Options controls path search costs and limits loaded from geoengine.properties.
type Options struct {
	MoveWeight      int
	MoveWeightDiag  int
	ObstacleWeight  int
	HeuristicWeight int
	MaxIterations   int
}

// DefaultOptions returns the shipped pathfinding defaults.
func DefaultOptions() Options {
	return Options{
		MoveWeight:      defaultMoveWeight,
		MoveWeightDiag:  defaultMoveWeightDiag,
		ObstacleWeight:  defaultObstacleWeight,
		HeuristicWeight: defaultHeuristicWeight,
		MaxIterations:   defaultMaxIterations,
	}
}

// OptionsFromProperties reads pathfinding options from geoengine.properties semantics.
func OptionsFromProperties(props *config.Properties) (Options, error) {
	if props == nil {
		return DefaultOptions(), nil
	}

	options := DefaultOptions()
	var err error

	if options.MoveWeight, err = props.Int("MoveWeight", options.MoveWeight); err != nil {
		return Options{}, fmt.Errorf("geo/pathfind: %w", err)
	}
	if options.MoveWeightDiag, err = props.Int("MoveWeightDiag", options.MoveWeightDiag); err != nil {
		return Options{}, fmt.Errorf("geo/pathfind: %w", err)
	}
	if options.ObstacleWeight, err = props.Int("ObstacleWeight", options.ObstacleWeight); err != nil {
		return Options{}, fmt.Errorf("geo/pathfind: %w", err)
	}
	if options.HeuristicWeight, err = props.Int("HeuristicWeight", options.HeuristicWeight); err != nil {
		return Options{}, fmt.Errorf("geo/pathfind: %w", err)
	}
	if options.MaxIterations, err = props.Int("MaxIterations", options.MaxIterations); err != nil {
		return Options{}, fmt.Errorf("geo/pathfind: %w", err)
	}
	return options, nil
}

func (o Options) obstacleWeightDiag() int {
	return int(float64(o.ObstacleWeight) * math.Sqrt2)
}
