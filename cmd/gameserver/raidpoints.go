package main

import (
	"context"
	"database/sql"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// provideRaidPoints returns the players' raid points, persisting through the
// gameserver database on worker's lanes.
func provideRaidPoints(db *sql.DB, worker *persist.Worker, log zerolog.Logger) *raidpoint.Points {
	return raidpoint.New(gamesql.NewRaidPointStore(db), worker, log)
}

// startRaidPoints restores the raid points before any character can log
// in. Their writes need no stop step: the persistence worker, stopped
// after the game server, lands whatever is still queued.
func startRaidPoints(lc fx.Lifecycle, points *raidpoint.Points) {
	lc.Append(fx.Hook{OnStart: points.Restore})
}

// startBossZones gives the players allowed into a boss zone before the
// last shutdown their permission back, and saves the allowed players once
// the game server has let every player go.
func startBossZones(lc fx.Lifecycle, data *gameData, db *sql.DB, log zerolog.Logger) {
	store := gamesql.NewBossZoneStore(db)
	zones := zone.OfKind[*zone.Boss](data.Zones)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return store.Restore(ctx, zones)
		},
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, shutdownSaveTimeout, log, "save boss zones", func(ctx context.Context) error {
				return store.SaveZones(ctx, zones)
			})
			return nil
		},
	})
}
