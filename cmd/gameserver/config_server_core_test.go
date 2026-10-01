package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
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
