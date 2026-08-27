package main

import (
	"context"
	"database/sql"
	"os"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/logging"
	"github.com/fatal10110/acis_golang/internal/config"
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
	for _, key := range cfg.UnsupportedKeys {
		rt.Logger.Warn().Str("file", "logging.properties").Str("key", key).Msg("config key has no Go reader")
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return rt.Close() }})
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

func provideIDAllocator(pool *sql.DB, log zerolog.Logger) (*idfactory.Allocator, error) {
	return idfactory.New(context.Background(), pool, log)
}
