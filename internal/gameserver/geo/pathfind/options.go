package pathfind

import (
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
	Bidirectional   bool
}

// DefaultOptions returns the shipped pathfinding defaults.
func DefaultOptions() Options {
	return Options{
		MoveWeight:      defaultMoveWeight,
		MoveWeightDiag:  defaultMoveWeightDiag,
		ObstacleWeight:  defaultObstacleWeight,
		HeuristicWeight: defaultHeuristicWeight,
		MaxIterations:   defaultMaxIterations,
		Bidirectional:   true,
	}
}

// OptionsFromProperties reads pathfinding options from geoengine.properties semantics.
func OptionsFromProperties(props *config.Properties) (Options, error) {
	if props == nil {
		return DefaultOptions(), nil
	}

	options := DefaultOptions()
	f := config.NewFields(props, "geo/pathfind")
	options.MoveWeight = f.Int("MoveWeight", options.MoveWeight)
	options.MoveWeightDiag = f.Int("MoveWeightDiag", options.MoveWeightDiag)
	options.ObstacleWeight = f.Int("ObstacleWeight", options.ObstacleWeight)
	options.HeuristicWeight = f.Int("HeuristicWeight", options.HeuristicWeight)
	options.MaxIterations = f.Int("MaxIterations", options.MaxIterations)
	options.Bidirectional = f.Bool("Bidirectional", options.Bidirectional)
	if err := f.Err(); err != nil {
		return Options{}, err
	}
	return options, nil
}

func (o Options) obstacleWeightDiag() int {
	return int(float64(o.ObstacleWeight) * math.Sqrt2)
}
