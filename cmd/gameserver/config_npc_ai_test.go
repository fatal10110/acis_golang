package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// TestLoadNpcAIConfigUsesNpcsProperties pins the npcs.properties
// MobAggroInPeaceZone (default True) and GuardAttackAggroMob (default False)
// switches, where any value other than a case-insensitive "true" reads as
// false, the DefaultSeeRange (default 450) see range and the MaxDriftRange
// (default 200) drift range.
func TestLoadNpcAIConfigUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want npc.AIConfig
	}{
		{name: "defaults", body: "", want: npc.AIConfig{MobAggroInPeaceZone: true, SeeRange: 450, DriftRange: 200}},
		{name: "shipped", body: "MobAggroInPeaceZone = True\nGuardAttackAggroMob = False\nDefaultSeeRange = 450\nMaxDriftRange = 200\n", want: npc.AIConfig{MobAggroInPeaceZone: true, SeeRange: 450, DriftRange: 200}},
		{name: "flipped", body: "MobAggroInPeaceZone = False\nGuardAttackAggroMob = TRUE\nDefaultSeeRange = 600\nMaxDriftRange = 350\n", want: npc.AIConfig{GuardAttackAggroMob: true, SeeRange: 600, DriftRange: 350}},
		{name: "not a boolean", body: "MobAggroInPeaceZone = yes\n", want: npc.AIConfig{SeeRange: 450, DriftRange: 200}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "npcs.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadNpcAIConfig(gameServerPaths{NpcsConfigPath: configPath})
			if err != nil {
				t.Fatalf("loadNpcAIConfig() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("loadNpcAIConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
