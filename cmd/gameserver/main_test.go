package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

func TestGameServerConfigFromProperties(t *testing.T) {
	serverProps, err := config.ParseString(`
Hostname = games.example.com
GameserverHostname = 127.0.0.3
GameserverPort = 17777
LoginHost = 127.0.0.4
LoginPort = 19014
RequestServerID = 7
AcceptAlternateID = False
MaximumOnlineUsers = 123
URL = jdbc:mariadb://db.example/acis
Login = acis
Password = secret
ServerGMOnly = True
ServerListClock = True
ServerListBrackets = True
ServerListAgeLimit = 18
TestServer = True
PvpServer = False
`)
	if err != nil {
		t.Fatalf("ParseString server: %v", err)
	}
	hexProps, err := config.ParseString(`
ServerID = 3
HexID = -7fff
`)
	if err != nil {
		t.Fatalf("ParseString hexid: %v", err)
	}

	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
	if err != nil {
		t.Fatalf("gameServerConfigFromProperties: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.3:17777" {
		t.Errorf("ListenAddr = %q, want 127.0.0.3:17777", cfg.ListenAddr)
	}
	if cfg.LoginAddr != "127.0.0.4:19014" {
		t.Errorf("LoginAddr = %q, want 127.0.0.4:19014", cfg.LoginAddr)
	}
	if cfg.Auth.ServerID != 3 {
		t.Errorf("Auth.ServerID = %d, want hexid ServerID 3", cfg.Auth.ServerID)
	}
	if cfg.Auth.AcceptAlternateID {
		t.Error("Auth.AcceptAlternateID = true, want false")
	}
	if cfg.Auth.HostName != "games.example.com" || cfg.Auth.Port != 17777 || cfg.Auth.MaxPlayers != 123 {
		t.Errorf("Auth advertised endpoint/capacity = %+v, want host games.example.com port 17777 max 123", cfg.Auth)
	}
	status := cfg.Auth.InitialStatus
	if status.Status == nil || *status.Status != link.ServerTypeGMOnly ||
		status.ShowClock == nil || !*status.ShowClock ||
		status.ShowBrackets == nil || !*status.ShowBrackets ||
		status.AgeLimit == nil || *status.AgeLimit != 18 ||
		status.TestServer == nil || !*status.TestServer ||
		status.Pvp == nil || *status.Pvp {
		t.Errorf("Auth.InitialStatus = %+v, want GMOnly clock/brackets age/test on and pvp off", status)
	}
	wantHex, err := model.ParseHexKey("-7fff")
	if err != nil {
		t.Fatalf("ParseHexKey: %v", err)
	}
	if !bytes.Equal(cfg.Auth.HexID, wantHex) {
		t.Errorf("Auth.HexID = %x, want %x", cfg.Auth.HexID, wantHex)
	}
	if cfg.Database.URL != "jdbc:mariadb://db.example/acis" || cfg.Database.Login != "acis" || cfg.Database.Password != "secret" {
		t.Errorf("Database = %+v, want parsed database credentials", cfg.Database)
	}
}

func TestLoadPvPFlagOptionsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte(`
PvPVsNormalTime = 1234
PvPVsPvPTime = 5678
KarmaPlayerCanShop = False
`), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, err := loadPvPFlagOptions(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPvPFlagOptions() error = %v", err)
	}
	if opts.Normal != 1234*time.Millisecond || opts.Flagged != 5678*time.Millisecond {
		t.Fatalf("durations = normal %s flagged %s, want 1234ms/5678ms", opts.Normal, opts.Flagged)
	}
	if len(opts.UnsupportedKeys) != 1 || opts.UnsupportedKeys[0] != "KarmaPlayerCanShop" {
		t.Fatalf("UnsupportedKeys = %v, want [KarmaPlayerCanShop]", opts.UnsupportedKeys)
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
	if !ok || got != "<html/>" {
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

	crests, err := loadCrestCache(gameServerPaths{DataRoot: root})
	if err != nil {
		t.Fatalf("loadCrestCache: %v", err)
	}
	got, ok := crests.Get(datacache.PledgeCrest, 101)
	if !ok || !bytes.Equal(got, data) {
		t.Fatalf("Get(PledgeCrest, 101) = %d bytes, %v; want cached crest", len(got), ok)
	}
}

func TestLoadCrestCacheAllowsMissingDirectory(t *testing.T) {
	crests, err := loadCrestCache(gameServerPaths{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("loadCrestCache: %v", err)
	}
	if crests.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 for missing crest directory", crests.Len())
	}
}

func TestWorldAttackStanceEffectsStopsPlayerRegistryActor(t *testing.T) {
	state := world.New()
	actor := &stoppableTestActor{id: 1001}
	state.AddPlayer(actor)

	worldAttackStanceEffects{state: state}.AutoAttackStop(actor)

	if !actor.stopped {
		t.Fatal("AutoAttackStop did not stop an actor present only in the player registry")
	}
}

type stoppableTestActor struct {
	world.Presence
	id      int32
	stopped bool
}

func (a *stoppableTestActor) ObjectID() int32 { return a.id }
func (a *stoppableTestActor) Stop()           { a.stopped = true }

func TestProvideAdditionalLifecycleTasks(t *testing.T) {
	water, err := provideWater()
	if err != nil {
		t.Fatalf("provideWater() error = %v", err)
	}
	if water == nil {
		t.Fatal("provideWater() = nil")
	}

	shadowItems, err := provideShadowItems()
	if err != nil {
		t.Fatalf("provideShadowItems() error = %v", err)
	}
	if shadowItems == nil {
		t.Fatal("provideShadowItems() = nil")
	}

	var _ *task.Water = water
	var _ *task.ShadowItems = shadowItems
}

func TestGameServerConfigUsesRequestIDWithoutHexID(t *testing.T) {
	serverProps, err := config.ParseString(`
GameserverHostname = *
GameserverPort = 7777
LoginHost = 127.0.0.1
LoginPort = 9014
RequestServerID = 9
`)
	if err != nil {
		t.Fatalf("ParseString server: %v", err)
	}

	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, nil)
	if err != nil {
		t.Fatalf("gameServerConfigFromProperties: %v", err)
	}

	if cfg.ListenAddr != ":7777" {
		t.Errorf("ListenAddr = %q, want :7777", cfg.ListenAddr)
	}
	if cfg.Auth.ServerID != 9 {
		t.Errorf("Auth.ServerID = %d, want RequestServerID 9", cfg.Auth.ServerID)
	}
	if !cfg.GeneratedHexID {
		t.Error("GeneratedHexID = false, want true when hexid file is missing")
	}
	if len(cfg.Auth.HexID) != generatedHexIDSize {
		t.Errorf("generated HexID length = %d, want %d", len(cfg.Auth.HexID), generatedHexIDSize)
	}
}

func TestLoadHexIDPropertiesAllowsMissingFile(t *testing.T) {
	props, err := loadHexIDProperties(gameServerPaths{HexIDPath: filepath.Join(t.TempDir(), "hexid.txt")})
	if err != nil {
		t.Fatalf("loadHexIDProperties() error = %v, want nil for missing file", err)
	}
	if props.Props != nil {
		t.Fatalf("Props = %v, want nil for missing file", props.Props)
	}
}

func TestGameServerConfigRejectsMaxPlayersOutsideInt32(t *testing.T) {
	serverProps, err := config.ParseString(`
MaximumOnlineUsers = 2147483648
`)
	if err != nil {
		t.Fatalf("ParseString server: %v", err)
	}

	if _, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, nil); err == nil {
		t.Fatalf("gameServerConfigFromProperties() error = nil, want range error above %d", int64(math.MaxInt32))
	}
}

func TestWriteHexIDFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "hexid.txt")
	key := []byte{0x80, 0x01}

	if err := writeHexIDFile(path, 3, key); err != nil {
		t.Fatalf("writeHexIDFile: %v", err)
	}

	props, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	serverID, err := props.Int("ServerID", 0)
	if err != nil {
		t.Fatalf("ServerID: %v", err)
	}
	if serverID != 3 {
		t.Fatalf("ServerID = %d, want 3", serverID)
	}
	gotHex := props.String("HexID", "")
	if want := model.HexKeyText(key); gotHex != want {
		t.Fatalf("HexID = %q, want %q", gotHex, want)
	}
	roundTrip, err := model.ParseHexKey(gotHex)
	if err != nil {
		t.Fatalf("ParseHexKey: %v", err)
	}
	if !bytes.Equal(roundTrip, key) {
		t.Fatalf("round-trip key = %x, want %x", roundTrip, key)
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

	geo, err := loadGeodata(gameServerPaths{DataRoot: dataRoot, GeoConfigPath: configPath})
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
	wantEngineOptions := engine.Options{MaxObstacleHeight: 48}
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

	geo, err := loadGeodata(gameServerPaths{DataRoot: dataRoot, GeoConfigPath: configPath})
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
