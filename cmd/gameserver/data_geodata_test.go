package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/rs/zerolog"
)

// writeGeoRegionL2OFF writes a full-size flat L2OFF region file.
func writeGeoRegionL2OFF(t *testing.T, dir string, x, y int, height int16) {
	t.Helper()
	buf := make([]byte, 18+block.RegionBlockCount*6)
	for i, off := 0, 18; i < block.RegionBlockCount; i, off = i+1, off+6 {
		binary.LittleEndian.PutUint16(buf[off+2:], uint16(height))
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d_%d_conv.dat", x, y)), buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeGeoConfig(t *testing.T, regions ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "geoengine.properties")
	body := "GeoDataPath = ./data/geodata/\nGeoDataType = L2OFF\n" + strings.Join(regions, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadGeodataFailsOnMissingListedRegion is #3315: boot must refuse to
// start when geoengine.properties lists a region whose file is absent, as the
// reference GeoEngine exits, instead of booting with a null region where
// every walk fails.
func TestLoadGeodataFailsOnMissingListedRegion(t *testing.T) {
	dataRoot := t.TempDir()
	geoDir := filepath.Join(dataRoot, "data", "geodata")
	if err := os.MkdirAll(geoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGeoRegionL2OFF(t, geoDir, 16, 10, 80)
	configPath := writeGeoConfig(t, "16_10", "17_25")

	_, err := loadGeodata(gameServerPaths{DataRoot: dataRoot, GeoConfigPath: configPath}, zerolog.Nop())
	if err == nil {
		t.Fatal("loadGeodata() error = nil, want failure for the missing listed region 17_25")
	}
	if msg := err.Error(); !strings.Contains(msg, "17_25") || !strings.Contains(msg, filepath.Join(geoDir, "17_25_conv.dat")) {
		t.Errorf("error %q does not name the missing region file", msg)
	}
}

// TestLoadGeodataLoadsOnlyListedRegions covers the complete-directory boot:
// a listed region loads, and a file on disk that is not listed stays null.
func TestLoadGeodataLoadsOnlyListedRegions(t *testing.T) {
	dataRoot := t.TempDir()
	geoDir := filepath.Join(dataRoot, "data", "geodata")
	if err := os.MkdirAll(geoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGeoRegionL2OFF(t, geoDir, 16, 10, 80)
	writeGeoRegionL2OFF(t, geoDir, 16, 11, 90)
	configPath := writeGeoConfig(t, "16_10", "#16_11")

	geo, err := loadGeodata(gameServerPaths{DataRoot: dataRoot, GeoConfigPath: configPath}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadGeodata: %v", err)
	}
	if got := geo.Engine.Height(engine.WorldX(0), engine.WorldY(0), 0); got != 80 {
		t.Errorf("Height() in listed 16_10 = %d, want 80", got)
	}
	// 16_11 starts one region (2048 geo cells) further along Y.
	if got := geo.Engine.Height(engine.WorldX(0), engine.WorldY(2048), 4321); got != 4321 {
		t.Errorf("Height() in unlisted 16_11 = %d, want unchanged worldZ 4321", got)
	}
}
