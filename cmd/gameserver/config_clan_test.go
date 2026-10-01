package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

// TestLoadClanConfigReadsLifeCrystalNeeded reads the clan skill item switch
// from players.properties, on by default as the reference's is.
func TestLoadClanConfigReadsLifeCrystalNeeded(t *testing.T) {
	dir := t.TempDir()
	clans := filepath.Join(dir, "clans.properties")
	if err := os.WriteFile(clans, []byte("DaysBeforeJoinAClan = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		players string
		want    bool
	}{
		{"LifeCrystalNeeded = False\n", false},
		{"LifeCrystalNeeded = True\n", true},
		{"", true},
	} {
		players := filepath.Join(dir, "players.properties")
		if err := os.WriteFile(players, []byte(tc.players), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadClanConfig(gameServerPaths{ClansConfigPath: clans, PlayersConfigPath: players}, zerolog.Nop())
		if err != nil {
			t.Fatalf("loadClanConfig(%q) error = %v", tc.players, err)
		}
		if cfg.LifeCrystalNeeded != tc.want || cfg.JoinDays != 2 {
			t.Fatalf("loadClanConfig(%q) = %+v, want LifeCrystalNeeded %v and 2 join days", tc.players, cfg, tc.want)
		}
	}
}
