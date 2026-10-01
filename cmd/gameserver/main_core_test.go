package main

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/config"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// TestGameServerGraphValidates checks the fx constructor graph resolves
// without a database: every provider's dependencies are satisfied by some
// other provider, with no missing or duplicate types. go build only proves
// the Go code compiles, not that dig can wire it; this runs on every
// change to newGameServerAppOptions instead of only failing at boot.
func TestGameServerGraphValidates(t *testing.T) {
	if err := fx.ValidateApp(newGameServerAppOptions(gameServerPaths{})...); err != nil {
		t.Fatalf("fx graph does not resolve: %v", err)
	}
}

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
AllowCursedWeapons = False
AllowWater = False
EnableFallingDamage = False
UseBlowfishCipher = False
ZoneTown = 2
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
	if cfg.Database.MaxConnections != 0 {
		t.Errorf("Database.MaxConnections = %d, want 0 (default pool size)", cfg.Database.MaxConnections)
	}
	if cfg.AllowCursedWeapons {
		t.Error("AllowCursedWeapons = true, want false")
	}
	if cfg.AllowWater {
		t.Error("AllowWater = true, want false")
	}
	if cfg.EnableFallingDamage {
		t.Error("EnableFallingDamage = true, want false")
	}
	if cfg.UseBlowfishCipher {
		t.Error("UseBlowfishCipher = true, want false")
	}
	if cfg.TownCombatRule != 2 {
		t.Errorf("TownCombatRule = %d, want ZoneTown 2", cfg.TownCombatRule)
	}
}

func TestGameServerConfigFromPropertiesMaxConnections(t *testing.T) {
	serverProps, err := config.ParseString(`
URL = jdbc:mariadb://localhost/acis
MaxConnections = 16
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
	if cfg.Database.MaxConnections != 16 {
		t.Errorf("Database.MaxConnections = %d, want 16", cfg.Database.MaxConnections)
	}
}

func TestGameServerConfigFromPropertiesAllowWaterDefaultsTrue(t *testing.T) {
	serverProps, err := config.ParseString("")
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
	if !cfg.AllowWater {
		t.Error("AllowWater = false, want true default")
	}
	if !cfg.EnableFallingDamage {
		t.Error("EnableFallingDamage = false, want true default")
	}
	if !cfg.UseBlowfishCipher {
		t.Error("UseBlowfishCipher = false, want true default")
	}
}

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

func TestLoadPvPFlagOptionsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte(`
PvPVsNormalTime = 1234
PvPVsPvPTime = 5678
CanGMDropEquipment = True
`), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, err := loadPvPFlagOptions(gameServerPaths{PlayersConfigPath: configPath}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadPvPFlagOptions() error = %v", err)
	}
	if opts.Normal != 1234*time.Millisecond || opts.Flagged != 5678*time.Millisecond {
		t.Fatalf("durations = normal %s flagged %s, want 1234ms/5678ms", opts.Normal, opts.Flagged)
	}
	if len(opts.UnsupportedKeys) != 1 || opts.UnsupportedKeys[0] != "CanGMDropEquipment" {
		t.Fatalf("UnsupportedKeys = %v, want [CanGMDropEquipment]", opts.UnsupportedKeys)
	}
}

func TestLoadAutoLearnSkillsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("AutoLearnSkills = True\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadAutoLearnSkills(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadAutoLearnSkills() error = %v", err)
	}
	if !got {
		t.Fatal("loadAutoLearnSkills() = false, want true")
	}
}

func TestLoadKarmaServiceGatesUsesPlayersProperties(t *testing.T) {
	dir := t.TempDir()
	set := filepath.Join(dir, "set.properties")
	if err := os.WriteFile(set, []byte("KarmaPlayerCanShop = True\nKarmaPlayerCanUseGK = True\nKarmaPlayerCanUseWareHouse = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unset := filepath.Join(dir, "unset.properties")
	if err := os.WriteFile(unset, []byte("AutoLearnSkills = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadKarmaServiceGates(gameServerPaths{PlayersConfigPath: set})
	if err != nil {
		t.Fatalf("loadKarmaServiceGates() error = %v", err)
	}
	if want := (karmaServiceGates{CanShop: true, CanUseGK: true, CanUseWareHouse: false}); got != want {
		t.Fatalf("loadKarmaServiceGates() = %+v, want %+v", got, want)
	}
	got, err = loadKarmaServiceGates(gameServerPaths{PlayersConfigPath: unset})
	if err != nil {
		t.Fatalf("loadKarmaServiceGates() defaults error = %v", err)
	}
	if want := (karmaServiceGates{CanShop: false, CanUseGK: false, CanUseWareHouse: true}); got != want {
		t.Fatalf("loadKarmaServiceGates() defaults = %+v, want %+v", got, want)
	}
}

// TestLoadKarmaPlayerCanTradeUsesPlayersProperties pins the trade karma
// switch: players.properties turns it off, and a file without the key keeps
// the shipped default of allowing karma trades.
func TestLoadKarmaPlayerCanTradeUsesPlayersProperties(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"KarmaPlayerCanTrade = False\n", false},
		{"KarmaPlayerCanTrade = True\n", true},
		{"KarmaPlayerCanShop = False\n", true},
	} {
		configPath := filepath.Join(t.TempDir(), "players.properties")
		if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadKarmaPlayerCanTrade(gameServerPaths{PlayersConfigPath: configPath})
		if err != nil {
			t.Fatalf("loadKarmaPlayerCanTrade(%q) error = %v", tc.body, err)
		}
		if bool(got) != tc.want {
			t.Fatalf("loadKarmaPlayerCanTrade(%q) = %v, want %v", tc.body, got, tc.want)
		}
	}
}

// TestLoadAllowDiscardItemUsesServerProperties pins the drop switch:
// server.properties turns it off, and a file without the key keeps the
// shipped default of allowing drops.
func TestLoadAllowDiscardItemUsesServerProperties(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"AllowDiscardItem = False\n", false},
		{"AllowDiscardItem = True\n", true},
		{"MultipleItemDrop = False\n", true},
	} {
		configPath := filepath.Join(t.TempDir(), "server.properties")
		if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadAllowDiscardItem(gameServerPaths{ConfigPath: configPath})
		if err != nil {
			t.Fatalf("loadAllowDiscardItem(%q) error = %v", tc.body, err)
		}
		if bool(got) != tc.want {
			t.Fatalf("loadAllowDiscardItem(%q) = %v, want %v", tc.body, got, tc.want)
		}
	}
}

func TestLoadDeathPenaltyChanceUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("DeathPenaltyChance = 73\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadDeathPenaltyChance(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadDeathPenaltyChance() error = %v", err)
	}
	if got != 73 {
		t.Fatalf("loadDeathPenaltyChance() = %d, want 73", got)
	}
}

func TestLoadDeathPenaltyChanceDefaultsToTwenty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadDeathPenaltyChance(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadDeathPenaltyChance() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("loadDeathPenaltyChance() = %d, want 20", got)
	}
}

func TestLoadMaxBuffsAmountUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MaxBuffsAmount = 24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMaxBuffsAmount(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMaxBuffsAmount() error = %v", err)
	}
	if got != 24 {
		t.Fatalf("loadMaxBuffsAmount() = %d, want 24", got)
	}
}

func TestLoadMaxBuffsAmountDefaultsToTwenty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMaxBuffsAmount(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMaxBuffsAmount() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("loadMaxBuffsAmount() = %d, want 20", got)
	}
}

func TestLoadSpawnMultiplierUsesNpcsProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "npcs.properties")
	if err := os.WriteFile(configPath, []byte("SpawnMultiplier = 1.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadSpawnMultiplier(gameServerPaths{NpcsConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadSpawnMultiplier() error = %v", err)
	}
	if got != 1.5 {
		t.Fatalf("loadSpawnMultiplier() = %v, want 1.5", got)
	}
}

func TestLoadSpawnMultiplierDefaultsToOne(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "npcs.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadSpawnMultiplier(gameServerPaths{NpcsConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadSpawnMultiplier() error = %v", err)
	}
	if got != 1 {
		t.Fatalf("loadSpawnMultiplier() = %v, want 1", got)
	}
}

// TestLoadRaidMultipliersUsesNpcsProperties pins Config.java:742-744: the
// three raid multipliers come from npcs.properties, each defaulting to 1.,
// and a malformed value fails the load as Double.parseDouble does.
func TestLoadRaidMultipliersUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		want    npc.RaidMultipliers
		wantErr bool
	}{
		{name: "defaults", body: "", want: npc.RaidMultipliers{Defence: 1, HPRegen: 1, MPRegen: 1}},
		{
			name: "configured",
			body: "RaidHpRegenMultiplier = 3.\nRaidMpRegenMultiplier = 0.5\nRaidDefenceMultiplier = 2\n",
			want: npc.RaidMultipliers{Defence: 2, HPRegen: 3, MPRegen: 0.5},
		},
		{name: "malformed", body: "RaidDefenceMultiplier = two\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadRaidMultipliers(gameServerPaths{NpcsConfigPath: configPath})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("loadRaidMultipliers() = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadRaidMultipliers() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("loadRaidMultipliers() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLoadDisableRaidCurseUsesNpcsProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "npcs.properties")
	if err := os.WriteFile(configPath, []byte("DisableRaidCurse = True\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadDisableRaidCurse(gameServerPaths{NpcsConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadDisableRaidCurse() error = %v", err)
	}
	if !got {
		t.Fatal("loadDisableRaidCurse() = false, want true")
	}
}

func TestLoadDisableRaidCurseDefaultsToFalse(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "npcs.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadDisableRaidCurse(gameServerPaths{NpcsConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadDisableRaidCurse() error = %v", err)
	}
	if got {
		t.Fatal("loadDisableRaidCurse() = true, want false")
	}
}

// TestLoadFreeTeleport pins npcs.properties FreeTeleport: false when
// absent or not "true", as read when set.
func TestLoadFreeTeleport(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       freeTeleport
		wantErr    bool
	}{
		{"absent", "", false, false},
		{"true", "FreeTeleport = True\n", true, false},
		{"false", "FreeTeleport = False\n", false, false},
		{"not a boolean", "FreeTeleport = maybe\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadFreeTeleport(gameServerPaths{NpcsConfigPath: configPath})
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("loadFreeTeleport() = %v, %v; want %v, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestLoadMaxGeoPathFailCountUsesGeoengineProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "geoengine.properties")
	if err := os.WriteFile(configPath, []byte("MaxGeopathFailCount = 80\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMaxGeoPathFailCount(gameServerPaths{GeoConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMaxGeoPathFailCount() error = %v", err)
	}
	if got != 80 {
		t.Fatalf("loadMaxGeoPathFailCount() = %d, want 80", got)
	}
}

func TestLoadMaxGeoPathFailCountDefaultsToFifty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "geoengine.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMaxGeoPathFailCount(gameServerPaths{GeoConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMaxGeoPathFailCount() error = %v", err)
	}
	if got != 50 {
		t.Fatalf("loadMaxGeoPathFailCount() = %d, want 50", got)
	}
}

func TestLoadMaxGeoPathFailCountFloorsAtFifteen(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "geoengine.properties")
	if err := os.WriteFile(configPath, []byte("MaxGeopathFailCount = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMaxGeoPathFailCount(gameServerPaths{GeoConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMaxGeoPathFailCount() error = %v", err)
	}
	if got != 15 {
		t.Fatalf("loadMaxGeoPathFailCount() = %d, want 15", got)
	}
}

func TestLoadRandomWalkRateUsesNpcsProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "npcs.properties")
	if err := os.WriteFile(configPath, []byte("RandomWalkRate = 45\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadRandomWalkRate(gameServerPaths{NpcsConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadRandomWalkRate() error = %v", err)
	}
	if got != 45 {
		t.Fatalf("loadRandomWalkRate() = %d, want 45", got)
	}
}

func TestLoadRandomWalkRateDefaultsToThirty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "npcs.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadRandomWalkRate(gameServerPaths{NpcsConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadRandomWalkRate() error = %v", err)
	}
	if got != 30 {
		t.Fatalf("loadRandomWalkRate() = %d, want 30", got)
	}
}

func TestLoadPerfectShieldBlockRateUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("PerfectShieldBlockRate = 15\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPerfectShieldBlockRate(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPerfectShieldBlockRate() error = %v", err)
	}
	if got != 15 {
		t.Fatalf("loadPerfectShieldBlockRate() = %d, want 15", got)
	}
}

func TestLoadPerfectShieldBlockRateDefaultsToFive(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPerfectShieldBlockRate(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPerfectShieldBlockRate() error = %v", err)
	}
	if got != 5 {
		t.Fatalf("loadPerfectShieldBlockRate() = %d, want 5", got)
	}
}

func TestLoadMagicFailuresUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MagicFailures = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMagicFailures(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMagicFailures() error = %v", err)
	}
	if got {
		t.Fatalf("loadMagicFailures() = %v, want false", got)
	}
}

func TestLoadMagicFailuresDefaultsToTrue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMagicFailures(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMagicFailures() error = %v", err)
	}
	if !got {
		t.Fatalf("loadMagicFailures() = %v, want true", got)
	}
}

func TestLoadCancelLesserEffectUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("CancelLesserEffect = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCancelLesserEffect(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadCancelLesserEffect() error = %v", err)
	}
	if got {
		t.Fatalf("loadCancelLesserEffect() = %v, want false", got)
	}
}

func TestLoadCancelLesserEffectDefaultsToTrue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCancelLesserEffect(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadCancelLesserEffect() error = %v", err)
	}
	if !got {
		t.Fatalf("loadCancelLesserEffect() = %v, want true", got)
	}
}

func TestLoadStoreSkillCooltimeUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("StoreSkillCooltime = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStoreSkillCooltime(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStoreSkillCooltime() error = %v", err)
	}
	if got {
		t.Fatalf("loadStoreSkillCooltime() = %v, want false", got)
	}
}

func TestLoadStoreSkillCooltimeDefaultsToTrue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStoreSkillCooltime(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStoreSkillCooltime() error = %v", err)
	}
	if !got {
		t.Fatalf("loadStoreSkillCooltime() = %v, want true", got)
	}
}

func TestLoadPlayerSpawnProtectionUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("PlayerSpawnProtection = 12\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPlayerSpawnProtection(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPlayerSpawnProtection() error = %v", err)
	}
	if got != playerSpawnProtection(12*time.Second) {
		t.Fatalf("loadPlayerSpawnProtection() = %v, want 12s", got)
	}
}

func TestLoadPlayerSpawnProtectionDefaultsToDisabled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPlayerSpawnProtection(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPlayerSpawnProtection() error = %v", err)
	}
	if got != playerSpawnProtection(0) {
		t.Fatalf("loadPlayerSpawnProtection() = %v, want disabled", got)
	}
}

func TestLoadCharacterSelectDelayUsesServerProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "server.properties")
	if err := os.WriteFile(configPath, []byte("CharacterSelectTime = 5000\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCharacterSelectDelay(gameServerPaths{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadCharacterSelectDelay() error = %v", err)
	}
	if got != characterSelectDelay(5*time.Second) {
		t.Fatalf("loadCharacterSelectDelay() = %v, want 5s", got)
	}
}

func TestLoadCharacterSelectDelayDefaultsToThreeSeconds(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "server.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCharacterSelectDelay(gameServerPaths{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadCharacterSelectDelay() error = %v", err)
	}
	if got != characterSelectDelay(3*time.Second) {
		t.Fatalf("loadCharacterSelectDelay() = %v, want 3s", got)
	}
}

func TestLoadServerBypassDelayUsesServerProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "server.properties")
	if err := os.WriteFile(configPath, []byte("ServerBypassTime = 250\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadServerBypassDelay(gameServerPaths{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadServerBypassDelay() error = %v", err)
	}
	if got != serverBypassDelay(250*time.Millisecond) {
		t.Fatalf("loadServerBypassDelay() = %v, want 250ms", got)
	}
}

// TestLoadCraftConfig pins the two craft knobs to their property keys and
// shipped defaults: ManufactureTime in server.properties (300 ms) and
// CraftingEnabled in players.properties (true).
func TestLoadCraftConfig(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.properties")
	set := filepath.Join(dir, "set.properties")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set, []byte("ManufactureTime = 1200\nCraftingEnabled = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, err := loadManufactureDelay(gameServerPaths{ConfigPath: empty}); err != nil || got != manufactureDelay(300*time.Millisecond) {
		t.Fatalf("loadManufactureDelay(default) = %v, %v; want 300ms", got, err)
	}
	if got, err := loadManufactureDelay(gameServerPaths{ConfigPath: set}); err != nil || got != manufactureDelay(1200*time.Millisecond) {
		t.Fatalf("loadManufactureDelay(set) = %v, %v; want 1.2s", got, err)
	}
	if got, err := loadCraftingEnabled(gameServerPaths{PlayersConfigPath: empty}); err != nil || !bool(got) {
		t.Fatalf("loadCraftingEnabled(default) = %v, %v; want true", got, err)
	}
	if got, err := loadCraftingEnabled(gameServerPaths{PlayersConfigPath: set}); err != nil || bool(got) {
		t.Fatalf("loadCraftingEnabled(set) = %v, %v; want false", got, err)
	}
}

// TestLoadMultisellConfig pins the two multisell knobs to their property
// keys and shipped defaults: MultisellTime in server.properties (100 ms)
// and BlacksmithUseRecipes in players.properties (true).
func TestLoadMultisellConfig(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.properties")
	set := filepath.Join(dir, "set.properties")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set, []byte("MultisellTime = 750\nBlacksmithUseRecipes = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, err := loadMultisellDelay(gameServerPaths{ConfigPath: empty}); err != nil || got != multisellDelay(100*time.Millisecond) {
		t.Fatalf("loadMultisellDelay(default) = %v, %v; want 100ms", got, err)
	}
	if got, err := loadMultisellDelay(gameServerPaths{ConfigPath: set}); err != nil || got != multisellDelay(750*time.Millisecond) {
		t.Fatalf("loadMultisellDelay(set) = %v, %v; want 750ms", got, err)
	}
	if got, err := loadBlacksmithUseRecipes(gameServerPaths{PlayersConfigPath: empty}); err != nil || !bool(got) {
		t.Fatalf("loadBlacksmithUseRecipes(default) = %v, %v; want true", got, err)
	}
	if got, err := loadBlacksmithUseRecipes(gameServerPaths{PlayersConfigPath: set}); err != nil || bool(got) {
		t.Fatalf("loadBlacksmithUseRecipes(set) = %v, %v; want false", got, err)
	}
}

func TestLoadServerBypassDelayDefaultsToHundredMilliseconds(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "server.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadServerBypassDelay(gameServerPaths{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadServerBypassDelay() error = %v", err)
	}
	if got != serverBypassDelay(100*time.Millisecond) {
		t.Fatalf("loadServerBypassDelay() = %v, want 100ms", got)
	}
}

func TestLoadPetConfigUsesServerAndPlayersProperties(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "server.properties")
	playersPath := filepath.Join(dir, "players.properties")
	if err := os.WriteFile(serverPath, []byte(`
PetXpRate = 1.5
SinEaterXpRate = 4.0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(playersPath, []byte(`
MaximumSlotsForPet = 19
WeightLimit = 1.25
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadPetConfig(gameServerPaths{ConfigPath: serverPath, PlayersConfigPath: playersPath}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadPetConfig() error = %v", err)
	}

	if got := cfg.ScaledExpGain(12077, 1000); got != 1500 {
		t.Errorf("ordinary pet configured exp = %d, want 1500", got)
	}
	if got := cfg.ScaledExpGain(12564, 1000); got != 4000 {
		t.Errorf("sin eater configured exp = %d, want 4000", got)
	}
	slots, _ := cfg.InventoryLimits(43)
	if slots != 19 {
		t.Errorf("pet inventory slots = %d, want 19", slots)
	}
	if cfg == pet.DefaultConfig() {
		t.Fatal("loadPetConfig returned defaults, want values from both files")
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

func TestGameServerConfigRejectsMissingHexIDProperties(t *testing.T) {
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

	if _, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, nil); err == nil {
		t.Fatal("gameServerConfigFromProperties() error = nil, want missing hexid properties error")
	}
}

func TestLoadHexIDPropertiesRejectsMissingFile(t *testing.T) {
	if _, err := loadHexIDProperties(gameServerPaths{HexIDPath: filepath.Join(t.TempDir(), "hexid.txt")}); err == nil {
		t.Fatal("loadHexIDProperties() error = nil, want missing-file error")
	}
}

func TestGameServerConfigRejectsHexIDFileMissingRequiredKey(t *testing.T) {
	serverProps, err := config.ParseString("RequestServerID = 9")
	if err != nil {
		t.Fatalf("ParseString server: %v", err)
	}

	for name, hexText := range map[string]string{
		"ServerID": "HexID = 0a",
		"HexID":    "ServerID = 9",
	} {
		t.Run(name, func(t *testing.T) {
			hexProps, err := config.ParseString(hexText)
			if err != nil {
				t.Fatalf("ParseString hexid: %v", err)
			}
			if _, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps); err == nil {
				t.Fatalf("gameServerConfigFromProperties() error = nil, want missing %s error", name)
			}
		})
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

// TestGameServerStopTimeoutCoversEveryStopStep pins fx's stop budget against
// the stop hooks this package actually registers. fx checks its one stop
// deadline before each hook and skips the rest once it has expired, and Run
// then exits the process with any running hook cut off, so one hook that can
// wait longer than its share (a slow database under a save, a backed-up
// persistence lane) drops every save after it.
//
// The hooks are found by parsing this package's source for every OnStop
// field, keyed by the function that registers it, so a new or moved hook
// fails here until its bound is recorded. A hook with its own budget must
// reference that budget's constant; a hook with no bound of its own says why
// it needs none and falls under gameServerStopSlack.
func TestGameServerStopTimeoutCoversEveryStopStep(t *testing.T) {
	type stopBound struct {
		bound time.Duration
		uses  string // constant the registering function must reference; "" when the bound lives elsewhere
		why   string
	}
	hooks := map[string]stopBound{
		"startGameServer": {
			network.LivePlayerPersistWait, "",
			"waits for the connection handlers; each exit waits at most LivePlayerPersistWait for its player's saves, in parallel, and cancelling closes the login link so its writes fail fast",
		},
		"startDebugHTTP":      {debugHTTPStopTimeout, "debugHTTPStopTimeout", "graceful stop of the debug listener"},
		"startNpcPersistence": {shutdownSaveTimeout, "shutdownSaveTimeout", "spawn_data save"},
		"startSimPool":        {simPoolStopTimeout, "simPoolStopTimeout", "actor pool finishing queued tasks"},
		"startTicker": {
			task.ItemInstanceSaveTimeout, "",
			"StopAndWait waits for one in-flight tick; the item tick is the only one with database I/O, bounded by ItemInstanceSaveTimeout, and the rest are in-memory",
		},
		"startItemInstances":         {3 * task.ItemInstanceSaveTimeout, "ItemInstanceSaveTimeout", "drainItemInstances: save, persistence-worker drain, save"},
		"providePersist":             {persistCloseTimeout, "persistCloseTimeout", "persistence worker's last close"},
		"startGroundItemPersistence": {shutdownSaveTimeout, "shutdownSaveTimeout", "items_on_ground save"},
		"startSevenSigns":            {0, "", "stops a timer under a lock the status save does not hold across its write"},
		"provideGameServerLogger":    {0, "", "closes the log file"},
		"provideBootContext":         {0, "", "cancels a context"},
		"provideGameServerDatabase":  {0, "", "closes the pool; the last database step, so running past the deadline loses nothing"},
	}

	registered := stopHookRegistrars(t)
	sum := gameServerStopSlack
	for name, b := range hooks {
		refs, ok := registered[name]
		if !ok {
			t.Errorf("%s is listed but registers no OnStop hook", name)
			continue
		}
		if b.uses != "" && !refs[b.uses] {
			t.Errorf("%s's stop hook does not reference %s, the bound it is budgeted for (%s)", name, b.uses, b.why)
		}
		sum += b.bound
	}
	for name := range registered {
		if _, ok := hooks[name]; !ok {
			t.Errorf("%s registers an OnStop hook with no recorded bound: give it its own budget and add it to gameServerStopTimeout", name)
		}
	}
	if gameServerStopTimeout < sum {
		t.Fatalf("gameServerStopTimeout = %s, below the %s its stop hooks can take", gameServerStopTimeout, sum)
	}
}

// stopHookRegistrars parses this package's non-test source and returns, for
// every function containing an OnStop field, the identifiers it references.
func stopHookRegistrars(t *testing.T) map[string]map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := make(map[string]map[string]bool)
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			refs := make(map[string]bool)
			hasStop := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.KeyValueExpr:
					if key, ok := n.Key.(*ast.Ident); ok && key.Name == "OnStop" {
						hasStop = true
					}
				case *ast.Ident:
					refs[n.Name] = true
				}
				return true
			})
			if hasStop {
				out[fn.Name.Name] = refs
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no OnStop hooks; is the test running in cmd/gameserver?")
	}
	return out
}

// TestSlowSpawnSaveLeavesItemDrainItsStopBudget stops an fx app whose hooks
// sit in the game server's order: the spawn-data save first, then the final
// item drain. The spawn save's database never answers and an owner's
// persistence lane is held past the drain's first save. The stop budget is
// the sum of the steps' own bounds, so fx still reaches the drain and the
// pending item row is written. A spawn save on fx's own stop context would
// spend the whole budget, and fx would then skip the drain.
func TestSlowSpawnSaveLeavesItemDrainItsStopBudget(t *testing.T) {
	const budget = 200 * time.Millisecond
	worker := persist.New(zerolog.Nop())
	flusher := &countingItemFlusher{}
	items := task.NewItemInstances(flusher, item.NewTable(nil), worker, nil, zerolog.Nop())
	inst := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 7, Count: 1, Location: item.LocationInventory}
	items.Add(inst)
	// Held through the spawn save and the drain's first save.
	worker.Enqueue(inst.OwnerID, func() { time.Sleep(2*budget + budget/2) })

	app := fx.New(fx.NopLogger, fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			return drainItemInstances(ctx, items, worker, zerolog.Nop(), budget)
		}})
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, budget, zerolog.Nop(), "save spawn data", func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
			return nil
		}})
	}))
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), budget+3*budget+budget)
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("stop error = %v, want every hook run inside the stop budget", err)
	}
	if got := flusher.count(); got != 1 {
		t.Fatalf("item flushes = %d, want 1 from the drain", got)
	}
	if items.Contains(inst) {
		t.Fatal("item still pending after shutdown")
	}
}

// TestSlowPersistCloseLeavesGroundSaveItsStopBudget stops an fx app whose
// hooks sit in the game server's order: the item drain, then the
// persistence worker's last close, then the ground-item save. One lane is
// backed up far past every budget, so both closes give up. Each close is
// bounded, so fx still reaches the ground-item save inside the stop budget.
// A close on fx's own stop context would wait out the whole budget, and fx
// would skip the save; items_on_ground was cleared at boot, so every ground
// item would be lost.
func TestSlowPersistCloseLeavesGroundSaveItsStopBudget(t *testing.T) {
	const budget = 100 * time.Millisecond
	worker := persist.New(zerolog.Nop())
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	worker.Enqueue(7, func() { <-release })
	items := task.NewItemInstances(&countingItemFlusher{}, item.NewTable(nil), worker, nil, zerolog.Nop())

	var groundSaved atomic.Bool
	app := fx.New(fx.NopLogger, fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, budget, zerolog.Nop(), "save ground items", func(context.Context) error {
				groundSaved.Store(true)
				return nil
			})
			return nil
		}})
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			return closePersistOnStop(ctx, worker, budget)
		}})
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			return drainItemInstances(ctx, items, worker, zerolog.Nop(), budget)
		}})
	}))
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 3*budget+budget+budget+2*budget)
	defer cancel()
	_ = app.Stop(stopCtx) // both closes report the backed-up lane
	if stopCtx.Err() != nil {
		t.Fatal("stop budget ran out before the last hook")
	}
	if !groundSaved.Load() {
		t.Fatal("ground-item save never ran")
	}
}

// TestDrainItemInstancesOutlivesExpiredStopContext runs the shutdown item
// drain with fx's stop ctx already expired and an owner's persistence lane
// held longer than one step's budget. The first save gives up behind the
// lane, and its owner job returns the item to pending when it runs. The drain
// must then wait for the worker on its own budget and write the item in the
// retry save: draining with the expired stop ctx returns at once and leaves
// the item unwritten.
func TestDrainItemInstancesOutlivesExpiredStopContext(t *testing.T) {
	worker := persist.New(zerolog.Nop())
	flusher := &countingItemFlusher{}
	items := task.NewItemInstances(flusher, item.NewTable(nil), worker, nil, zerolog.Nop())
	inst := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 7, Count: 1, Location: item.LocationInventory}
	items.Add(inst)

	const budget = 200 * time.Millisecond
	worker.Enqueue(inst.OwnerID, func() { time.Sleep(budget + budget/2) })
	stopCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := drainItemInstances(stopCtx, items, worker, zerolog.Nop(), budget); err != nil {
		t.Fatalf("drain error = %v", err)
	}
	if got := flusher.count(); got != 1 {
		t.Fatalf("item flushes = %d, want 1 from the retry save after the drain", got)
	}
	if items.Contains(inst) {
		t.Fatal("item still pending after the drain")
	}
}

type countingItemFlusher struct {
	mu sync.Mutex
	n  int
}

func (f *countingItemFlusher) Flush(context.Context, item.FlushBatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	return nil
}

func (f *countingItemFlusher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func TestLoadInventorySlotsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MaximumSlotsForNoDwarf = 90\nMaximumSlotsForDwarf = 117\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadInventorySlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadInventorySlots() error = %v", err)
	}
	if got != (player.InventorySlots{NoDwarf: 90, Dwarf: 117, Configured: true}) {
		t.Fatalf("loadInventorySlots() = %+v, want {NoDwarf:90 Dwarf:117}", got)
	}
}

// TestLoadInventorySlotsKeepsExplicitZero pins the reference's as-is read:
// an explicit 0 is a 0-slot base, not the shipped default.
func TestLoadInventorySlotsKeepsExplicitZero(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MaximumSlotsForNoDwarf = 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadInventorySlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadInventorySlots() error = %v", err)
	}
	if got != (player.InventorySlots{NoDwarf: 0, Dwarf: 100, Configured: true}) {
		t.Fatalf("loadInventorySlots() = %+v, want {NoDwarf:0 Dwarf:100 Configured:true}", got)
	}
}

func TestLoadInventorySlotsDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadInventorySlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadInventorySlots() error = %v", err)
	}
	if got != player.DefaultInventorySlots {
		t.Fatalf("loadInventorySlots() = %+v, want {NoDwarf:80 Dwarf:100}", got)
	}
}

func TestLoadStorageSlotsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	props := "MaximumWarehouseSlotsForNoDwarf = 11\nMaximumWarehouseSlotsForDwarf = 12\n" +
		"MaximumFreightSlots = 13\nMaxPvtStoreSlotsOther = 14\nMaxPvtStoreSlotsDwarf = 15\n" +
		"DwarfRecipeLimit = 16\nCommonRecipeLimit = 0\n"
	if err := os.WriteFile(configPath, []byte(props), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStorageSlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStorageSlots() error = %v", err)
	}
	want := player.StorageSlots{
		WarehouseNoDwarf: 11, WarehouseDwarf: 12, Freight: 13,
		PrivateStoreNoDwarf: 14, PrivateStoreDwarf: 15,
		DwarfRecipe: 16, CommonRecipe: 0, Configured: true,
	}
	if got != want {
		t.Fatalf("loadStorageSlots() = %+v, want %+v", got, want)
	}
}

func TestLoadStorageSlotsDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStorageSlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStorageSlots() error = %v", err)
	}
	if got != player.DefaultStorageSlots {
		t.Fatalf("loadStorageSlots() = %+v, want %+v", got, player.DefaultStorageSlots)
	}
}

func TestLoadEnchantConfigUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte(`
EnchantChanceMagicWeapon = 0.5
EnchantChanceMagicWeapon15Plus = 0.25
EnchantChanceNonMagicWeapon = 0.8
EnchantChanceNonMagicWeapon15Plus = 0.45
EnchantChanceArmor = 0.7
EnchantMaxWeapon = 16
EnchantMaxArmor = 12
EnchantSafeMax = 4
EnchantSafeMaxFull = 5
`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadEnchantConfig(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadEnchantConfig() error = %v", err)
	}
	want := enchant.Config{
		ChanceMagicWeapon: 0.5, ChanceMagicWeapon15Plus: 0.25,
		ChanceWeapon: 0.8, ChanceWeapon15Plus: 0.45, ChanceArmor: 0.7,
		MaxWeapon: 16, MaxArmor: 12, SafeMax: 4, SafeMaxFull: 5,
	}
	if got != want {
		t.Fatalf("loadEnchantConfig() = %+v, want %+v", got, want)
	}

	empty := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = loadEnchantConfig(gameServerPaths{PlayersConfigPath: empty})
	if err != nil {
		t.Fatalf("loadEnchantConfig(empty) error = %v", err)
	}
	if got != enchant.DefaultConfig() {
		t.Fatalf("loadEnchantConfig(empty) = %+v, want the shipped defaults %+v", got, enchant.DefaultConfig())
	}
}
