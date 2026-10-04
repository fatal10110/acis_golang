package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// TestLoadNpcAIConfigUsesNpcsProperties pins the npcs.properties
// MobAggroInPeaceZone (default True) and GuardAttackAggroMob (default False)
// switches; any value other than a case-insensitive "true" reads as false.
func TestLoadNpcAIConfigUsesNpcsProperties(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want npc.AIConfig
	}{
		{name: "defaults", body: "", want: npc.AIConfig{MobAggroInPeaceZone: true}},
		{name: "shipped", body: "MobAggroInPeaceZone = True\nGuardAttackAggroMob = False\n", want: npc.AIConfig{MobAggroInPeaceZone: true}},
		{name: "flipped", body: "MobAggroInPeaceZone = False\nGuardAttackAggroMob = TRUE\n", want: npc.AIConfig{GuardAttackAggroMob: true}},
		{name: "not a boolean", body: "MobAggroInPeaceZone = yes\n", want: npc.AIConfig{}},
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
