package main

import (
	"context"
	"database/sql"

	"github.com/fatal10110/acis_golang/internal/config"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// loadOlympiadConfig reads the events.properties Olympiad calendar
// settings, defaulting as the reference does when a key is missing.
func loadOlympiadConfig(paths gameServerPaths) (olympiad.Config, error) {
	props, err := config.LoadFile(paths.EventsConfigPath)
	if err != nil {
		return olympiad.Config{}, err
	}
	f := config.NewFields(props, "events")
	def := olympiad.DefaultConfig()
	cfg := olympiad.Config{
		StartHour:         f.Int("OlyStartTime", def.StartHour),
		StartMinute:       f.Int("OlyMin", def.StartMinute),
		CompetitionMillis: f.Int64("OlyCPeriod", def.CompetitionMillis),
		WeeklyPoints:      f.Int("OlyWeeklyPoints", def.WeeklyPoints),
	}
	return cfg, f.Err()
}

// provideOlympiad returns the Olympiad, persisting through the gameserver
// database on worker's lanes, announcing to every player in state and running its calendar
// on pool.
func provideOlympiad(paths gameServerPaths, db *sql.DB, pool *sim.Pool, worker *persist.Worker, state *world.State, log zerolog.Logger) (*olympiad.Olympiad, error) {
	cfg, err := loadOlympiadConfig(paths)
	if err != nil {
		return nil, err
	}
	return olympiad.New(cfg, gamesql.NewOlympiadStore(db), worker, network.NewOlympiadAnnouncer(state), pool.NewQueue("olympiad"), log), nil
}

// startOlympiad restores the cycle and the nobles' records before any
// character can log in and starts the calendar; on shutdown it stops the
// calendar and saves them.
func startOlympiad(lc fx.Lifecycle, o *olympiad.Olympiad) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := o.Restore(ctx); err != nil {
				return err
			}
			o.Start()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// Waits for a calendar step still running, then for its
			// queued writes and the final save; the persist worker,
			// stopped after this hook, lands whatever is left.
			ctx, cancel := context.WithTimeout(ctx, 2*olympiad.TaskTimeout)
			defer cancel()
			o.Stop(ctx)
			return nil
		},
	})
}
