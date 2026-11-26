package app

import (
	"context"
	"database/sql"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// PersistCloseTimeout bounds the persistence worker's last close, which
// gives lanes the item drain's own close left backed up a little longer.
// The ground-item save and the database close still follow it.
const PersistCloseTimeout = 5 * time.Second

// NewPersistWorker starts the persistence worker that runs player, pet and
// item writes. It takes the pool so fx builds the database first and
// therefore closes it only after this worker has drained, and every hook
// appended later (item flush, the game listener's detach saves) stops
// before the drain.
func NewPersistWorker(lc fx.Lifecycle, _ *sql.DB, log zerolog.Logger) *persist.Worker {
	worker := persist.New(log)
	lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		return ClosePersistWorker(ctx, worker, PersistCloseTimeout)
	}})
	return worker
}

// ClosePersistWorker waits at most budget of the stop context for the
// persistence worker to drain. Lanes a slow database has backed up would
// otherwise hold the rest of the stop budget, and fx would then skip the
// ground-item save that stops after this hook.
func ClosePersistWorker(ctx context.Context, worker *persist.Worker, budget time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return worker.Close(ctx)
}
