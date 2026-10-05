package main

import (
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/config"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// loadManor reads the manor seed rows, each with the reference prices of
// its seed and crop items (1 for an item not loaded), and the manor areas.
// An area whose nodes do not triangulate is left out with a warning; the
// rest still load.
func loadManor(xmlRoot string, items *item.Table, log zerolog.Logger) (*manor.Table, *manor.AreaIndex, error) {
	seeds, err := gamexml.LoadManors(filepath.Join(xmlRoot, "manors.xml"))
	if err != nil {
		return nil, nil, err
	}
	seeds.ApplyReferencePrices(func(id int32) (int32, bool) {
		tmpl, ok := items.Get(id)
		if !ok {
			return 0, false
		}
		return tmpl.ReferencePrice, true
	})
	areas, err := gamexml.LoadManorAreas(filepath.Join(xmlRoot, "manorAreas.xml"))
	if err != nil {
		return nil, nil, err
	}
	index, skipped := manor.NewAreaIndex(areas)
	for _, err := range skipped {
		log.Warn().Err(err).Msg("cannot load manor area")
	}
	log.Info().Int("seeds", len(seeds.SeedsByID)).Int("manor_areas", index.Len()).Msg("manor data loaded")
	return seeds, index, nil
}

// manorSettings are the server.properties manor switches: AllowManor turns
// the seed and harvester items on, RateDropManor multiplies every harvested
// crop count.
type manorSettings struct {
	Allowed  bool
	CropRate int
}

func loadManorSettings(paths gameServerPaths) (manorSettings, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return manorSettings{}, err
	}
	f := config.NewFields(props, "manor")
	settings := manorSettings{
		Allowed:  f.Bool("AllowManor", true),
		CropRate: f.Int("RateDropManor", 1),
	}
	if err := f.Err(); err != nil {
		return manorSettings{}, err
	}
	return settings, nil
}

// manorConfig is the manor the game client link reads.
func manorConfig(settings manorSettings, data *gameData) network.ManorConfig {
	return network.ManorConfig{
		Allowed:  settings.Allowed,
		CropRate: settings.CropRate,
		Seeds:    data.Manors,
		Areas:    data.ManorAreas,
	}
}
