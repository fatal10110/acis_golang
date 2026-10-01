package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/rs/zerolog"
)

func TestApplyTownCombatRuleUsesZoneTown(t *testing.T) {
	form, err := zone.NewCuboid(0, 1, 0, 1, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	town, err := zone.NewTown(1, form, commons.NewStatSet())
	if err != nil {
		t.Fatal(err)
	}
	index := zone.NewIndex()
	index.Add(town)

	applyTownCombatRule(index, 2)

	if town.CombatRule != 2 {
		t.Fatalf("Town.CombatRule = %d, want ZoneTown 2", town.CombatRule)
	}
}

func TestLoadHTMLCacheUsesDatapackRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data", "html", "help", "tutorial.htm")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<html/>"), 0o600); err != nil {
		t.Fatal(err)
	}

	html, err := loadHTMLCache(gameServerPaths{DataRoot: root})
	if err != nil {
		t.Fatalf("loadHTMLCache: %v", err)
	}
	got, ok := html.Get("help/tutorial.htm")
	if !ok || got != "<html/>\n" {
		t.Fatalf("Get(help/tutorial.htm) = %q, %v; want cached html", got, ok)
	}
}

func TestLoadCrestCacheUsesDatapackRoot(t *testing.T) {
	root := t.TempDir()
	data := bytes.Repeat([]byte{0x5a}, 256)
	path := filepath.Join(root, "data", "crests", "Crest_101.dds")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	crests, err := loadCrestCache(gameServerPaths{DataRoot: root}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadCrestCache: %v", err)
	}
	got, ok := crests.Get(datacache.PledgeCrest, 101)
	if !ok || !bytes.Equal(got, data) {
		t.Fatalf("Get(PledgeCrest, 101) = %d bytes, %v; want cached crest", len(got), ok)
	}
}

func TestLoadCrestCacheAllowsMissingDirectory(t *testing.T) {
	crests, err := loadCrestCache(gameServerPaths{DataRoot: t.TempDir()}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadCrestCache: %v", err)
	}
	if crests.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 for missing crest directory", crests.Len())
	}
}

func TestLoadGeodataUsesGeoengineProperties(t *testing.T) {
	dataRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataRoot, "data", "geodata"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "geoengine.properties")
	if err := os.WriteFile(configPath, []byte(`
GeoDataPath = ./data/geodata/
GeoDataType = L2J
MoveWeight = 11
MoveWeightDiag = 15
ObstacleWeight = 33
HeuristicWeight = 17
MaxIterations = 1234
MaxObstacleHeight = 48
`), 0o600); err != nil {
		t.Fatal(err)
	}

	geo, err := loadGeodata(gameServerPaths{DataRoot: dataRoot, GeoConfigPath: configPath}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadGeodata: %v", err)
	}

	if geo.Engine == nil {
		t.Fatal("Engine = nil, want loaded geodata engine")
	}
	if geo.Finder == nil {
		t.Fatal("Finder = nil, want pathfinder over loaded engine")
	}
	if got, want := filepath.Clean(geo.Dir), filepath.Join(dataRoot, "data", "geodata"); got != want {
		t.Errorf("Dir = %q, want %q", got, want)
	}
	if geo.Type != probe.L2J {
		t.Errorf("Type = %q, want %q", geo.Type, probe.L2J)
	}
	wantEngineOptions := engine.Options{MaxObstacleHeight: 48, PartOfCharacterHeight: 75}
	if geo.EngineOptions != wantEngineOptions {
		t.Errorf("EngineOptions = %#v, want %#v", geo.EngineOptions, wantEngineOptions)
	}
	if got := geo.Engine.MaxObstacleHeight(); got != 48 {
		t.Errorf("Engine.MaxObstacleHeight() = %d, want 48", got)
	}
	wantOptions := pathfind.Options{
		MoveWeight:      11,
		MoveWeightDiag:  15,
		ObstacleWeight:  33,
		HeuristicWeight: 17,
		MaxIterations:   1234,
		Bidirectional:   true,
	}
	if geo.Pathfind != wantOptions {
		t.Errorf("Pathfind = %#v, want %#v", geo.Pathfind, wantOptions)
	}
}

func TestLoadGeodataDefaultsToDatapackGeodata(t *testing.T) {
	dataRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "geoengine.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	geo, err := loadGeodata(gameServerPaths{DataRoot: dataRoot, GeoConfigPath: configPath}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadGeodata: %v", err)
	}

	if got, want := filepath.Clean(geo.Dir), filepath.Join(dataRoot, "data", "geodata"); got != want {
		t.Errorf("Dir = %q, want %q", got, want)
	}
	if geo.Type != probe.L2OFF {
		t.Errorf("Type = %q, want %q", geo.Type, probe.L2OFF)
	}
	if geo.EngineOptions != engine.DefaultOptions() {
		t.Errorf("EngineOptions = %#v, want defaults %#v", geo.EngineOptions, engine.DefaultOptions())
	}
	if got := geo.Engine.MaxObstacleHeight(); got != engine.DefaultOptions().MaxObstacleHeight {
		t.Errorf("Engine.MaxObstacleHeight() = %d, want default", got)
	}
	if geo.Pathfind != pathfind.DefaultOptions() {
		t.Errorf("Pathfind = %#v, want defaults %#v", geo.Pathfind, pathfind.DefaultOptions())
	}
}

// ---- from data_test.go ----
// TestLogUnsupportedSkillEffectsWarnsOnAnUnmappedCoreEffect is #1517's
// regression case for its "log it once at load" ask: before #1517, a
// coreKinds gap like the pre-fix Signet family degraded every affected
// cast to a silent no-op with nothing in the logs. This proves the boot
// warning actually fires for an unmapped name and stays quiet for a real
// one, so a future coreKinds gap doesn't repeat that silent-degradation
// pattern undetected.
func TestLogUnsupportedSkillEffectsWarnsOnAnUnmappedCoreEffect(t *testing.T) {
	table := skill.NewTable([]skill.Definition{
		{
			ID:    9999,
			Level: 1,
			Name:  "Definitely Not A Real Effect Kind",
			Effects: []skill.EffectTemplate{
				{Name: "ThisEffectKindDoesNotExist"},
			},
		},
	})

	var buf bytes.Buffer
	logUnsupportedSkillEffects(table, zerolog.New(&buf))

	got := buf.String()
	if !strings.Contains(got, "ThisEffectKindDoesNotExist") {
		t.Fatalf("log output = %q, want a warning naming the unmapped effect", got)
	}
}

func TestLogUnsupportedSkillEffectsStaysQuietForAKnownCoreEffect(t *testing.T) {
	table := skill.NewTable([]skill.Definition{
		{
			ID:      9998,
			Level:   1,
			Name:    "Real Buff",
			Effects: []skill.EffectTemplate{{Name: "Buff"}},
		},
	})

	var buf bytes.Buffer
	logUnsupportedSkillEffects(table, zerolog.New(&buf))

	if got := buf.String(); got != "" {
		t.Fatalf("log output = %q, want no warning for a recognized core effect", got)
	}
}

// TestLogUnsupportedSkillEffectsStaysQuietForAnUnresolvedTableName covers
// the #1516 loader defect (an unresolved "#table" reference name): that's
// a separate, already-tracked bug, not a coreKinds gap, so this helper
// must not warn about it.
func TestLogUnsupportedSkillEffectsStaysQuietForAnUnresolvedTableName(t *testing.T) {
	table := skill.NewTable([]skill.Definition{
		{
			ID:      9997,
			Level:   1,
			Name:    "Unresolved Table Ref",
			Effects: []skill.EffectTemplate{{Name: "#effectname1"}},
		},
	})

	var buf bytes.Buffer
	logUnsupportedSkillEffects(table, zerolog.New(&buf))

	if got := buf.String(); got != "" {
		t.Fatalf("log output = %q, want no warning for the #1516 unresolved-table-name defect", got)
	}
}
