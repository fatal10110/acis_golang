package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
)

// TestLoadPetitionConfig pins the petition settings to players.properties
// with their shipped defaults: petitioning on, five petitions per player
// and 25 active at once.
func TestLoadPetitionConfig(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name, props string
		want        petition.Config
	}{
		{"default", "", petition.Config{Allowed: true, MaxPerPlayer: 5, MaxPending: 25}},
		{
			"set", "PetitioningAllowed = False\nMaxPetitionsPerPlayer = 2\nMaxPetitionsPending = 9\n",
			petition.Config{MaxPerPlayer: 2, MaxPending: 9},
		},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadPetitionConfig(gameServerPaths{PlayersConfigPath: path})
		if err != nil || got != tt.want {
			t.Errorf("%s: loadPetitionConfig = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}
