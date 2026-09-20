package main

import (
	"context"
	"database/sql"
	"os"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/logging"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

func provideGameServerLogger(lc fx.Lifecycle, paths gameServerPaths) (zerolog.Logger, error) {
	props, err := config.LoadFile(paths.LoggingPath)
	if err != nil {
		return zerolog.Logger{}, err
	}
	cfg, err := logging.ConfigFromProperties(props)
	if err != nil {
		return zerolog.Logger{}, err
	}
	rt, err := logging.Setup(paths.LogRoot, cfg, os.Stderr)
	if err != nil {
		return zerolog.Logger{}, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return rt.Close() }})
	// Config warnings are raised lazily while properties are read, so route
	// them here rather than leaving them on the unconfigured stderr logger.
	config.SetLogger(rt.Logger)
	return rt.Logger, nil
}

func provideGameServerDatabase(lc fx.Lifecycle, cfg gameServerConfig) (*sql.DB, error) {
	pool, err := db.Open(cfg.Database)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error { return pool.PingContext(ctx) },
		OnStop:  func(context.Context) error { return pool.Close() },
	})
	return pool, nil
}

// providePersist starts the persistence worker that runs player, pet and item
// writes. It takes the pool so fx builds the database first and therefore
// closes it only after this worker has drained, and every hook appended later
// (item flush, the game listener's detach saves) stops before the drain.
func providePersist(lc fx.Lifecycle, _ *sql.DB, log zerolog.Logger) *persist.Worker {
	worker := persist.New(log)
	lc.Append(fx.Hook{OnStop: worker.Close})
	return worker
}

// provideItemWriteOrder builds the ordering both writers of the items table
// share: the handlers' single-row writes and the lazy persistence task write
// the same rows from different lanes, and a row's last write has to be the
// last one produced for it.
func provideItemWriteOrder() *persist.Order {
	return persist.NewOrder()
}

func provideIDAllocator(ctx bootContext, pool *sql.DB, log zerolog.Logger) (*idfactory.Allocator, error) {
	return idfactory.New(ctx, pool, log)
}
