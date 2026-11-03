package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadShowNPCLevelUsesNpcsProperties pins npcs.properties ShowNpcLevel:
// read as set, false when absent.
func TestLoadShowNPCLevelUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       npcLevelShown
	}{
		{"set", "ShowNpcLevel = True\n", true},
		{"cleared", "ShowNpcLevel = False\n", false},
		{"absent", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadShowNPCLevel(gameServerPaths{NpcsConfigPath: configPath})
			if err != nil {
				t.Fatalf("loadShowNPCLevel() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("loadShowNPCLevel() = %v, want %v", got, tc.want)
			}
		})
	}
}
