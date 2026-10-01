package main

import (
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
)

// loadPetitionConfig reads the players.properties petition settings:
// whether players may petition (PetitioningAllowed), how many petitions one
// player may send (MaxPetitionsPerPlayer) and how many may be active at
// once (MaxPetitionsPending).
func loadPetitionConfig(paths gameServerPaths) (petition.Config, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return petition.Config{}, err
	}
	def := petition.DefaultConfig()
	f := config.NewFields(props, "petition")
	cfg := petition.Config{
		Allowed:      f.Bool("PetitioningAllowed", def.Allowed),
		MaxPerPlayer: f.Int("MaxPetitionsPerPlayer", def.MaxPerPlayer),
		MaxPending:   f.Int("MaxPetitionsPending", def.MaxPending),
	}
	return cfg, f.Err()
}
