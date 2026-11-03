package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/rs/zerolog"
)

// TestLoadGeodataServesListedRegionsFromCache covers the boot wiring of the
// geodata cache: the first boot builds the cache file from the listed
// regions, and a later boot serves the same answers from it.
func TestLoadGeodataServesListedRegionsFromCache(t *testing.T) {
	dataRoot := t.TempDir()
	geoDir := filepath.Join(dataRoot, "data", "geodata")
	if err := os.MkdirAll(geoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGeoRegionL2OFF(t, geoDir, 16, 10, 80)
	paths := gameServerPaths{
		DataRoot:      dataRoot,
		GeoConfigPath: writeGeoConfig(t, "16_10"),
		GeoCachePath:  filepath.Join(t.TempDir(), "geodata.cache"),
	}

	for boot := range 2 {
		geo, err := loadGeodata(paths, zerolog.Nop())
		if err != nil {
			t.Fatalf("boot %d: loadGeodata: %v", boot, err)
		}
		if _, err := os.Stat(paths.GeoCachePath); err != nil {
			t.Fatalf("boot %d: cache file: %v", boot, err)
		}
		if got := geo.Engine.Height(engine.WorldX(0), engine.WorldY(0), 0); got != 80 {
			t.Errorf("boot %d: Height() in listed 16_10 = %d, want 80", boot, got)
		}
	}
}
