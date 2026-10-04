package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

// Reference: Config.java loads DaysToPassToDissolveAClan from
// clans.properties, 7 when the key is missing; 0 dissolves a clan at once.
func TestLoadClanConfigDissolveDays(t *testing.T) {
	for _, tc := range []struct {
		props string
		want  int
	}{
		{"", 7},
		{"DaysToPassToDissolveAClan = 3\n", 3},
		{"DaysToPassToDissolveAClan = 0\n", 0},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "clans.properties")
		if err := os.WriteFile(path, []byte(tc.props), 0o600); err != nil {
			t.Fatal(err)
		}
		players := filepath.Join(dir, "players.properties")
		if err := os.WriteFile(players, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadClanConfig(gameServerPaths{ClansConfigPath: path, PlayersConfigPath: players}, zerolog.Nop())
		if err != nil {
			t.Fatalf("loadClanConfig(%q) error = %v", tc.props, err)
		}
		if got.DissolveDays != tc.want {
			t.Fatalf("loadClanConfig(%q).DissolveDays = %d, want %d", tc.props, got.DissolveDays, tc.want)
		}
	}
}
