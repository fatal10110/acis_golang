package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestLoadSpawnEvents pins Config.SPAWN_EVENTS: npcs.properties SpawnEvents
// split on whitespace, commas and semicolons, defaulting to
// extra_mob;18age;start_weapon when the key is missing.
func TestLoadSpawnEvents(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"shipped", "SpawnEvents = extra_mob;18age;start_weapon\n", []string{"extra_mob", "18age", "start_weapon"}},
		{"mixed delimiters", "SpawnEvents = 18age, christmas ;medal\n", []string{"18age", "christmas", "medal"}},
		{"missing", "SpawnMultiplier = 1.\n", []string{"extra_mob", "18age", "start_weapon"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadSpawnEvents(gameServerPaths{NpcsConfigPath: path})
			if err != nil {
				t.Fatalf("loadSpawnEvents() error = %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("loadSpawnEvents() = %q, want %q", got, tc.want)
			}
		})
	}
}
