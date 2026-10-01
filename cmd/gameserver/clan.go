package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// loadClanConfig reads the clans.properties penalties the clan core uses,
// defaulting as the reference does when a key is missing.
func loadClanConfig(paths gameServerPaths, _ zerolog.Logger) (clan.Config, error) {
	props, err := config.LoadFile(paths.ClansConfigPath)
	if err != nil {
		return clan.Config{}, err
	}
	f := config.NewFields(props, "clans")
	cfg := clan.Config{
		JoinDays:   f.Int("DaysBeforeJoinAClan", 5),
		CreateDays: f.Int("DaysBeforeCreateAClan", 10),
	}
	return cfg, f.Err()
}

// provideClans restores every clan from the database, after the id factory
// has dropped the clans whose leader no longer exists, clears the crest ids
// whose image the crest cache does not hold, and returns the clan service
// writing through the persistence worker.
func provideClans(ctx bootContext, pool *sql.DB, ids *idfactory.Allocator, worker *persist.Worker, cfg clan.Config, crests *datacache.Crests, log zerolog.Logger) (*clan.Service, error) {
	store := gamesql.NewClanStore(pool)
	snap, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("restore clans: %w", err)
	}
	table := clan.NewTable()
	table.Restore(snap, time.Now(), cfg.JoinDays)
	log.Info().Int("clans", table.Len()).Msg("clans loaded")
	service := clan.NewService(table, store, worker, ids, cfg, time.Now, log)
	service.DropMissingCrests(crests)
	return service, nil
}
