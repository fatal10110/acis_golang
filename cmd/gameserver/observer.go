package main

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/rs/zerolog"
)

// startObserverTowers places the broadcasting towers into the NPC
// population built at boot.
func startObserverTowers(npcs *manager.Npcs, data *gameData, log zerolog.Logger) {
	placed := npcs.SpawnObservers(data.Observers)
	log.Info().Int("observer_groups", data.Observers.GroupCount()).Int("observer_spawns", placed).Msg("observer groups loaded")
}
