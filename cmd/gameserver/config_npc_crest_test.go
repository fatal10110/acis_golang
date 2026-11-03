package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadShowNPCCrestUsesNpcsProperties pins npcs.properties ShowNpcCrest:
// read as set, false when absent.
func TestLoadShowNPCCrestUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       npcCrestShown
	}{
		{"set", "ShowNpcCrest = True\n", true},
		{"cleared", "ShowNpcCrest = False\n", false},
		{"absent", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadShowNPCCrest(gameServerPaths{NpcsConfigPath: configPath})
			if err != nil {
				t.Fatalf("loadShowNPCCrest() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("loadShowNPCCrest() = %v, want %v", got, tc.want)
			}
		})
	}
}
