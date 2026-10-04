package main

import (
	"context"
	"database/sql"

	"github.com/fatal10110/acis_golang/internal/config"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// loadFishingChampionshipConfig reads the events.properties
// AllowFishChampionship switch and FishChampionship* prizes, each
// defaulting to its shipped value when a key is missing. A malformed value
// fails boot.
func loadFishingChampionshipConfig(paths gameServerPaths) (fishchamp.Config, error) {
	events, err := config.LoadFile(paths.EventsConfigPath)
	if err != nil {
		return fishchamp.Config{}, err
	}
	def := fishchamp.DefaultConfig()
	f := config.NewFields(events, "events")
	cfg := fishchamp.Config{
		Enabled:      f.Bool("AllowFishChampionship", def.Enabled),
		RewardItemID: int32Setting(f, "FishChampionshipRewardItemId", def.RewardItemID),
	}
	for i, key := range [fishchamp.Places]string{"FishChampionshipReward1", "FishChampionshipReward2", "FishChampionshipReward3", "FishChampionshipReward4", "FishChampionshipReward5"} {
		cfg.Rewards[i] = int32Setting(f, key, def.Rewards[i])
	}
	return cfg, f.Err()
}

// provideFishingChampionship returns the fishing championship, saving
// through the gameserver database on worker's lanes and running its
// calendar on pool.
func provideFishingChampionship(paths gameServerPaths, db *sql.DB, pool *sim.Pool, worker *persist.Worker, log zerolog.Logger) (*fishchamp.Championship, error) {
	cfg, err := loadFishingChampionshipConfig(paths)
	if err != nil {
		return nil, err
	}
	return fishchamp.New(cfg, gamesql.NewFishingChampionshipStore(db), worker, pool.NewQueue("fishchamp"), log), nil
}

// startFishingChampionship restores the championship before any character
// can log in and ends a week found over; on shutdown it stops the calendar
// and saves the championship. The save waits for one already running on
// the persistence lane, bounded by fishchamp.TaskTimeout, then has its own
// shutdownSaveTimeout.
func startFishingChampionship(lc fx.Lifecycle, c *fishchamp.Championship, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := c.Restore(ctx); err != nil {
				return err
			}
			c.Start()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, fishchamp.TaskTimeout+shutdownSaveTimeout, log, "save fishing championship", c.Stop)
			return nil
		},
	})
}
