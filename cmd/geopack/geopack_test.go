package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
	"github.com/rs/zerolog"
)

// TestRunWritesCacheAndReport packs the regions a geoengine.properties lists
// and reports their stored size; the game server then serves the regions
// from that file without rebuilding it.
func TestRunWritesCacheAndReport(t *testing.T) {
	dir := t.TempDir()
	region := make([]byte, 18+block.RegionBlockCount*6)
	for off := 18; off < len(region); off += 6 {
		binary.LittleEndian.PutUint16(region[off+2:], 40)
	}
	if err := os.WriteFile(filepath.Join(dir, "16_10_conv.dat"), region, 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "geoengine.properties")
	if err := os.WriteFile(config, []byte("GeoDataPath = "+dir+"\nGeoDataType = L2OFF\n16_10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "geodata.cache")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", config, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "regions 1: flat 65536, complex 0, multilayer 0 blocks") {
		t.Fatalf("report = %s", stdout.String())
	}

	var logs bytes.Buffer
	e, err := probe.LoadRegionsCached(dir, probe.L2OFF, []probe.Region{{X: 16, Y: 10}}, out, zerolog.New(&logs))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "rebuilding") {
		t.Fatalf("boot rebuilt the packed cache: %s", logs.String())
	}
	if got := e.Height(engine.WorldX(0), engine.WorldY(0), 0); got != 40 {
		t.Fatalf("Height() = %d, want 40", got)
	}
}

// TestRunResolvesGeoDataPathLikeTheServer packs a config with an empty
// GeoDataPath: the regions come from <data-root>/data/geodata, as the game
// server reads them, not from geopack's working directory.
func TestRunResolvesGeoDataPathLikeTheServer(t *testing.T) {
	root := t.TempDir()
	geodata := filepath.Join(root, "data", "geodata")
	if err := os.MkdirAll(geodata, 0o755); err != nil {
		t.Fatal(err)
	}
	region := make([]byte, 18+block.RegionBlockCount*6)
	for off := 18; off < len(region); off += 6 {
		binary.LittleEndian.PutUint16(region[off+2:], 72)
	}
	if err := os.WriteFile(filepath.Join(geodata, "16_10_conv.dat"), region, 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "geoengine.properties")
	if err := os.WriteFile(config, []byte("GeoDataPath =\nGeoDataType = L2OFF\n16_10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "geodata.cache")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", config, "-data-root", root, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr %s", code, stderr.String())
	}

	var logs bytes.Buffer
	e, err := probe.LoadRegionsCached(probe.ResolveDir(root, ""), probe.L2OFF, []probe.Region{{X: 16, Y: 10}}, out, zerolog.New(&logs))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "rebuilding") {
		t.Fatalf("boot rebuilt the packed cache: %s", logs.String())
	}
	if got := e.Height(engine.WorldX(0), engine.WorldY(0), 0); got != 72 {
		t.Fatalf("Height() = %d, want 72", got)
	}
}
