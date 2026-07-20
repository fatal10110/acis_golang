package engine

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/config"
)

const (
	defaultMaxObstacleHeight     = 32
	defaultPartOfCharacterHeight = 75
)

// Options controls engine behavior loaded from geoengine.properties.
type Options struct {
	MaxObstacleHeight int

	// PartOfCharacterHeight is the percentage of an actor's full body
	// height (collision height * 2) used as its line-of-sight eye level.
	PartOfCharacterHeight int
}

// DefaultOptions returns the shipped geo-engine defaults.
func DefaultOptions() Options {
	return Options{MaxObstacleHeight: defaultMaxObstacleHeight, PartOfCharacterHeight: defaultPartOfCharacterHeight}
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
	if options.PartOfCharacterHeight, err = props.Int("PartOfCharacterHeight", options.PartOfCharacterHeight); err != nil {
		return Options{}, fmt.Errorf("geo/engine: %w", err)
	}
	return options, nil
}
