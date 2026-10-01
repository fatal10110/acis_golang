package main

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/logging"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// gmAuditLogger is the gmaudit log sink, apart from the process logger.
type gmAuditLogger zerolog.Logger

// enabled returns the audit sink when on is set (server.properties
// GMAudit), else the zero logger, which records nothing.
func (g gmAuditLogger) enabled(on bool) zerolog.Logger {
	if !on {
		return zerolog.Logger{}
	}
	return zerolog.Logger(g)
}

func provideGameServerLogger(lc fx.Lifecycle, paths gameServerPaths) (zerolog.Logger, gmAuditLogger, error) {
	props, err := config.LoadFile(paths.LoggingPath)
	if err != nil {
		return zerolog.Logger{}, gmAuditLogger{}, err
	}
	cfg, err := logging.ConfigFromProperties(props)
	if err != nil {
		return zerolog.Logger{}, gmAuditLogger{}, err
	}
	rt, err := logging.Setup(paths.LogRoot, cfg, os.Stderr)
	if err != nil {
		return zerolog.Logger{}, gmAuditLogger{}, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return rt.Close() }})
	// Config warnings are raised lazily while properties are read, so route
	// them here rather than leaving them on the unconfigured stderr logger.
	config.SetLogger(rt.Logger)
	return rt.Logger, gmAuditLogger(rt.GMAudit), nil
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
	lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		return closePersistOnStop(ctx, worker, persistCloseTimeout)
	}})
	return worker
}

// closePersistOnStop waits at most budget of the stop context for the
// persistence worker to drain. Lanes a slow database has backed up would
// otherwise hold the rest of the stop budget, and fx would then skip the
// ground-item save that stops after this hook.
func closePersistOnStop(ctx context.Context, worker *persist.Worker, budget time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return worker.Close(ctx)
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
