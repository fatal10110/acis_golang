package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// BugLogFile is the name of the geodata bug report file in the geodata
// directory.
const BugLogFile = "geo_bugs.txt"

// BugLog appends game masters' geodata bug reports to the report file of
// a directory, one line per report, opening the file on the first report.
// It is safe for concurrent use.
type BugLog struct {
	dir string
	mu  sync.Mutex
	f   *os.File
}

// NewBugLog returns the bug log of dir.
func NewBugLog(dir string) *BugLog {
	return &BugLog{dir: dir}
}

// Add appends the report of comment at world position x, y, z, creating
// the report file when missing.
func (b *BugLog) Add(x, y, z int, comment string) error {
	line := BugReport(x, y, z, comment)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.f == nil {
		f, err := os.OpenFile(filepath.Join(b.dir, BugLogFile), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			return fmt.Errorf("geo/engine: open bug log: %w", err)
		}
		b.f = f
	}
	_, err := io.WriteString(b.f, line)
	return err
}

// Close closes the report file, if a report opened it.
func (b *BugLog) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.f == nil {
		return nil
	}
	err := b.f.Close()
	b.f = nil
	return err
}

// BugReport is the bug report line of comment at world position x, y, z:
// the region, the block in it and the cell in that, then z and comment,
// separated by ';', any ';' in comment written as ':'.
func BugReport(x, y, z int, comment string) string {
	gx, gy := GeoX(x), GeoY(y)
	return fmt.Sprintf("%d;%d;%d;%d;%d;%d;%d;%s\r\n",
		gx/regionCellsX+TileXMin, gy/regionCellsY+TileYMin,
		(gx/block.CellsX)%(regionCellsX/block.CellsX), (gy/block.CellsY)%(regionCellsY/block.CellsY),
		gx%block.CellsX, gy%block.CellsY,
		z, strings.ReplaceAll(comment, ";", ":"))
}

// RegionOf is the geodata region holding world position x, y.
func RegionOf(x, y int) (int, int) {
	return GeoX(x)/regionCellsX + TileXMin, GeoY(y)/regionCellsY + TileYMin
}

// BlockName names the layout of the block holding geodata cell geoX, geoY,
// as game masters are shown it. A block a geodata object overlays, now or
// at any time before, is named for the layered cells an overlay keeps.
func (e *Engine) BlockName(geoX, geoY int) string {
	b := e.blockAtGeo(geoX, geoY)
	switch {
	case b.dyn != nil && b.dyn.Kind() == block.KindMultilayer:
		return "BlockMultilayerDynamic"
	case b.dyn != nil:
		return "BlockComplexDynamic"
	case b.region == nil:
		return "BlockNull"
	}
	if multilayer, ok := e.overlaid.Load(blockKey{geoX / block.CellsX, geoY / block.CellsY}); ok {
		if multilayer.(bool) {
			return "BlockMultilayerDynamic"
		}
		return "BlockComplexDynamic"
	}
	switch b.static().Kind() {
	case block.KindFlat:
		return "BlockFlat"
	case block.KindComplex:
		return "BlockComplex"
	case block.KindMultilayer:
		return "BlockMultilayer"
	}
	return "BlockNull"
}
