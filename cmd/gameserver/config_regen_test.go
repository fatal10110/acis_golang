package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// TestLoadRegenMultipliersUsesPlayersProperties pins Config.java:767-769:
// the three regeneration multipliers come from players.properties, each
// defaulting to 1., and a malformed value fails the load as
// Double.parseDouble does.
func TestLoadRegenMultipliersUsesPlayersProperties(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		want    creature.RegenMultipliers
		wantErr bool
	}{
		{name: "defaults", body: "", want: creature.RegenMultipliers{HP: 1, MP: 1, CP: 1, Configured: true}},
		{
			name: "configured",
			body: "HpRegenMultiplier = 2.\nMpRegenMultiplier = 0.5\nCpRegenMultiplier = 0\n",
			want: creature.RegenMultipliers{HP: 2, MP: 0.5, CP: 0, Configured: true},
		},
		{name: "malformed", body: "CpRegenMultiplier = fast\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "players.properties")
			if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadRegenMultipliers(gameServerPaths{PlayersConfigPath: configPath})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("loadRegenMultipliers() = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadRegenMultipliers() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("loadRegenMultipliers() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
