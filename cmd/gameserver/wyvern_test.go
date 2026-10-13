package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// TestLoadWyvernRulesUsesNpcsProperties pins npcs.properties
// RequiredStriderLevel and RequiredCrystalsNumber: read as set, 55 and 10
// when absent, and a malformed value failing boot.
func TestLoadWyvernRulesUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       npc.WyvernRules
		fails      bool
	}{
		{name: "set", body: "RequiredStriderLevel = 60\nRequiredCrystalsNumber = 25\n", want: npc.WyvernRules{StriderLevel: 60, Crystals: 25}},
		{name: "absent", want: npc.WyvernRules{StriderLevel: 55, Crystals: 10}},
		{name: "malformed", body: "RequiredCrystalsNumber = many\n", fails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadWyvernRules(gameServerPaths{NpcsConfigPath: configPath})
			if tc.fails {
				if err == nil {
					t.Fatalf("loadWyvernRules() = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadWyvernRules() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("loadWyvernRules() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
