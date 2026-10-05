package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// loadCastleManorConfig reads the manor cycle: the server.properties
// switch and crop rate settings carries, and the clans.properties
// Manor* times, each defaulting to its shipped value when a key is
// missing. A malformed value fails boot.
func loadCastleManorConfig(paths gameServerPaths, settings manorSettings) (castlemanor.Config, error) {
	props, err := config.LoadFile(paths.ClansConfigPath)
	if err != nil {
		return castlemanor.Config{}, err
	}
	def := castlemanor.DefaultConfig()
	f := config.NewFields(props, "clans")
	cfg := castlemanor.Config{
		Enabled:        settings.Allowed,
		CropRate:       settings.CropRate,
		RefreshHour:    f.Int("ManorRefreshTime", def.RefreshHour),
		RefreshMin:     f.Int("ManorRefreshMin", def.RefreshMin),
		ApproveHour:    f.Int("ManorApproveTime", def.ApproveHour),
		ApproveMin:     f.Int("ManorApproveMin", def.ApproveMin),
		MaintenanceMin: f.Int("ManorMaintenanceMin", def.MaintenanceMin),
		SavePeriod:     time.Duration(f.Int("ManorSavePeriodRate", int(def.SavePeriod/time.Hour))) * time.Hour,
	}
	return cfg, f.Err()
}

// provideCastleManor restores the castles' manor lists, once the castles
// are restored, saving through the persistence worker.
func provideCastleManor(ctx bootContext, paths gameServerPaths, gameplay gameplayConfig, pool *sql.DB, worker *persist.Worker, castles *castle.Manager, clans *clan.Service, data *gameData, log zerolog.Logger) (*castlemanor.Manager, error) {
	cfg, err := loadCastleManorConfig(paths, gameplay.Manor)
	if err != nil {
		return nil, err
	}
	m := castlemanor.New(cfg, data.Manors, castles, clans.Table(), gamesql.NewManorStore(pool), worker, log)
	if err := m.Restore(ctx); err != nil {
		return nil, fmt.Errorf("restore manor: %w", err)
	}
	return m, nil
}

// startCastleManor runs the manor cycle, paying the castle owners' clans
// and telling their leaders through link, before any character can log
// in; on shutdown it stops the cycle and saves the lists. The save waits
// for one already running on the persistence lane, bounded by
// castlemanor.TaskTimeout, then has its own shutdownSaveTimeout.
func startCastleManor(lc fx.Lifecycle, m *castlemanor.Manager, link *network.GameClientLink, pool *sim.Pool, log zerolog.Logger) {
	queue := pool.NewQueue("castle-manor")
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			m.Start(queue, network.CastleManorEffects(link))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, castlemanor.TaskTimeout+shutdownSaveTimeout, log, "save manor", m.Stop)
			return nil
		},
	})
}
