package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

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
