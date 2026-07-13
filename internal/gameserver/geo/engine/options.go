package engine

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/config"
)

const defaultMaxObstacleHeight = 32

// Options controls engine behavior loaded from geoengine.properties.
type Options struct {
	MaxObstacleHeight int
}

// DefaultOptions returns the shipped geo-engine defaults.
func DefaultOptions() Options {
	return Options{MaxObstacleHeight: defaultMaxObstacleHeight}
}

// OptionsFromProperties reads engine options from geoengine.properties semantics.
func OptionsFromProperties(props *config.Properties) (Options, error) {
	if props == nil {
		return DefaultOptions(), nil
	}

	options := DefaultOptions()
	var err error
	if options.MaxObstacleHeight, err = props.Int("MaxObstacleHeight", options.MaxObstacleHeight); err != nil {
		return Options{}, fmt.Errorf("geo/engine: %w", err)
	}
	return options, nil
}
