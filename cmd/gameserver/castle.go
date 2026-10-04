package main

import (
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// provideCastles restores every castle's stored state and owner, after the
// clans: an owner row naming a clan that is not loaded is skipped. Each
// change is then written through the persistence worker.
func provideCastles(ctx bootContext, pool *sql.DB, worker *persist.Worker, clans *clan.Service, data *gameData, log zerolog.Logger) (*castle.Manager, error) {
	store := gamesql.NewCastleStore(pool)
	rows, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("restore castles: %w", err)
	}
	owners, err := store.LoadOwners(ctx)
	if err != nil {
		return nil, fmt.Errorf("restore castles: %w", err)
	}
	castles := castle.NewManager(data.Castles, clans.Table(), store, worker, log)
	castles.Restore(rows, owners)
	log.Info().Int("castles", len(castles.All())).Msg("castles loaded")
	return castles, nil
}
