package main

import (
	"context"
	"database/sql"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/config"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// loadWeddingConfig reads the npcs.properties WeddingPrice (default
// 1000000), WeddingAllowSameSex (default false) and WeddingFormalWear
// (default true) keys. A malformed price fails boot.
func loadWeddingConfig(paths gameServerPaths) (wedding.Config, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return wedding.Config{}, err
	}
	def := wedding.DefaultConfig()
	f := config.NewFields(props, "wedding")
	cfg := wedding.Config{
		Price:      f.Int("WeddingPrice", def.Price),
		SameSex:    f.Bool("WeddingAllowSameSex", def.SameSex),
		FormalWear: f.Bool("WeddingFormalWear", def.FormalWear),
	}
	if err := f.Err(); err != nil {
		return wedding.Config{}, err
	}
	return cfg, nil
}

// provideWedding loads the couples stored in mods_wedding, numbering new
// ones from the id allocator, whose boot scan already counts the stored
// couple ids as used. A load that fails stops boot, so that the shutdown
// save cannot replace couples it never read.
func provideWedding(ctx bootContext, paths gameServerPaths, pool *sql.DB, ids *idfactory.Allocator, log zerolog.Logger) (*wedding.Manager, *gamesql.CoupleStore, error) {
	cfg, err := loadWeddingConfig(paths)
	if err != nil {
		return nil, nil, err
	}
	store := gamesql.NewCoupleStore(pool)
	couples, err := store.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	log.Info().Int("count", len(couples)).Msg("wedding: loaded couples")
	return wedding.NewManager(cfg, ids, couples), store, nil
}

// startWedding stores every couple once the game server has let every
// player go: the couples are written at shutdown only.
func startWedding(lc fx.Lifecycle, couples *wedding.Manager, store *gamesql.CoupleStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, shutdownSaveTimeout, log, "save couples", func(ctx context.Context) error {
				return store.Save(ctx, couples.Couples())
			})
			return nil
		},
	})
}
