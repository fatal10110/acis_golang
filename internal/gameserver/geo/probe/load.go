package probe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/reader"
)

// GeoType selects which geodata region-file format LoadEngine reads.
type GeoType string

const (
	L2OFF GeoType = "L2OFF"
	L2J   GeoType = "L2J"
)

// LoadEngine builds a geo Engine from every region file present in dir,
// named per geoType's convention (L2OFF: "x_y_conv.dat"; L2J: "x_y.l2j").
// A region tile with no file on disk is left unloaded, answered by the
// engine's Null-block fallback, the same as any never-configured region.
func LoadEngine(dir string, geoType GeoType) (*engine.Engine, error) {
	if geoType != L2OFF && geoType != L2J {
		return nil, fmt.Errorf("probe: unknown geodata type %q", geoType)
	}

	e := engine.New()
	for x := engine.TileXMin; x <= engine.TileXMax; x++ {
		for y := engine.TileYMin; y <= engine.TileYMax; y++ {
			blocks, err := readRegion(regionPath(dir, geoType, x, y), geoType)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("probe: load region %d_%d: %w", x, y, err)
			}
			if err := e.SetRegion(x, y, blocks); err != nil {
				return nil, fmt.Errorf("probe: load region %d_%d: %w", x, y, err)
			}
		}
	}
	return e, nil
}

func regionPath(dir string, geoType GeoType, x, y int) string {
	if geoType == L2J {
		return filepath.Join(dir, fmt.Sprintf("%d_%d.l2j", x, y))
	}
	return filepath.Join(dir, fmt.Sprintf("%d_%d_conv.dat", x, y))
}

func readRegion(path string, geoType GeoType) ([]block.Block, error) {
	if geoType == L2J {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return reader.ReadL2J(f)
	}
	return reader.ReadL2OFF(path)
}
