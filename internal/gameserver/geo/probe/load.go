package probe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/reader"
	"github.com/rs/zerolog"
)

// GeoType selects which geodata region-file format LoadEngine reads.
type GeoType string

const (
	L2OFF GeoType = "L2OFF"
	L2J   GeoType = "L2J"
)

// Region names one geodata region tile by its file coordinates.
type Region struct {
	X, Y int
}

// ListedRegions returns the region tiles geoengine.properties enables, in
// tile order: every in-range tile whose exact "X_Y" key is present, whatever
// its value. This is the reference GeoEngine's props.containsKey(rx + "_" + ry)
// check; a commented-out "#X_Y" line is no key and leaves the tile unlisted.
func ListedRegions(props *config.Properties) []Region {
	if props == nil {
		return nil
	}
	var regions []Region
	for x := engine.TileXMin; x <= engine.TileXMax; x++ {
		for y := engine.TileYMin; y <= engine.TileYMax; y++ {
			if _, ok := props.Lookup(fmt.Sprintf("%d_%d", x, y)); ok {
				regions = append(regions, Region{X: x, Y: y})
			}
		}
	}
	return regions
}

// LoadRegions builds a geo Engine from exactly the listed region files, as
// the reference GeoEngine constructor does at boot: every listed region is
// loaded from dir, every other tile keeps the engine's Null-block fallback
// even when its file is on disk. It logs the loaded count, then fails when
// any listed region could not be read or decoded (missing, unreadable, or
// malformed), naming each failed file plus dir and geoType, where the
// reference logs the failed count and exits the process.
func LoadRegions(dir string, geoType GeoType, regions []Region, log zerolog.Logger, options ...engine.Options) (*engine.Engine, error) {
	if !validGeoType(geoType) {
		return nil, fmt.Errorf("probe: unknown geodata type %q", geoType)
	}

	e := engine.New(options...)
	loaded := 0
	var failures []string
	for _, r := range regions {
		path := regionPath(dir, geoType, r.X, r.Y)
		if err := loadRegion(e, path, geoType, r.X, r.Y, log); err != nil {
			log.Error().Err(err).Str("region_file", path).Msg("error loading geodata region file")
			failures = append(failures, fmt.Sprintf("%d_%d (%s): %v", r.X, r.Y, path, err))
			continue
		}
		loaded++
	}
	log.Info().Int("region_files", loaded).Str("geodata_type", string(geoType)).Msg("geodata region files loaded")

	if len(failures) > 0 {
		return nil, fmt.Errorf("probe: failed to load %d %s region files listed in geoengine.properties (GeoDataPath %s, GeoDataType %s); the server will not start until every listed region loads: %s",
			len(failures), geoType, dir, geoType, strings.Join(failures, "; "))
	}
	return e, nil
}

// LoadEngine builds a geo Engine from every region file present in dir,
// named per geoType's convention (L2OFF: "x_y_conv.dat"; L2J: "x_y.l2j").
// A region tile with no file on disk is left unloaded, answered by the
// engine's Null-block fallback, the same as any never-configured region.
// It is geoprobe's "load whatever exists" mode; the gameserver boot loads the
// geoengine.properties region list through LoadRegions instead.
func LoadEngine(dir string, geoType GeoType, log zerolog.Logger, options ...engine.Options) (*engine.Engine, error) {
	if !validGeoType(geoType) {
		return nil, fmt.Errorf("probe: unknown geodata type %q", geoType)
	}

	e := engine.New(options...)
	for x := engine.TileXMin; x <= engine.TileXMax; x++ {
		for y := engine.TileYMin; y <= engine.TileYMax; y++ {
			err := loadRegion(e, regionPath(dir, geoType, x, y), geoType, x, y, log)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("probe: load region %d_%d: %w", x, y, err)
			}
		}
	}
	return e, nil
}

func validGeoType(geoType GeoType) bool {
	return geoType == L2OFF || geoType == L2J
}

// loadRegion reads one region file into e. A file with unread trailing bytes
// still loads, with a warning, as in the reference.
func loadRegion(e *engine.Engine, path string, geoType GeoType, x, y int, log zerolog.Logger) error {
	blocks, trailing, err := readRegion(path, geoType)
	if err != nil {
		return err
	}
	if trailing > 0 {
		log.Warn().Int("region_x", x).Int("region_y", y).Int("trailing_bytes", trailing).Msg("geodata region has trailing bytes")
	}
	return e.SetRegion(x, y, blocks)
}

func regionPath(dir string, geoType GeoType, x, y int) string {
	if geoType == L2J {
		return filepath.Join(dir, fmt.Sprintf("%d_%d.l2j", x, y))
	}
	return filepath.Join(dir, fmt.Sprintf("%d_%d_conv.dat", x, y))
}

// readRegion returns the region alongside the count of unread trailing bytes
// in its file, which only the L2OFF format reports.
func readRegion(path string, geoType GeoType) (*block.Region, int, error) {
	if geoType == L2J {
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		defer f.Close()
		region, err := reader.ReadL2J(f)
		return region, 0, err
	}
	return reader.ReadL2OFF(path)
}
