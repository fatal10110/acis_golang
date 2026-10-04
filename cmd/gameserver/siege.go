package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// siegeKeysNotApplied are the siege.properties keys this server reads but
// does not apply yet, each with the issue that applies it.
var siegeKeysNotApplied = []struct{ key, issue string }{
	{"AttackerRespawn", "#3346"},
	{"ChSiegeClanMinLevel", "#244"},
	{"ChAttackerMaxClans", "#244"},
}

// loadSiegeConfig reads the siege.properties castle siege settings,
// defaulting as the reference does when a key is missing. It also returns
// the keys set in the file that have no effect yet, so the boot can say
// so: AttackerRespawn waits for the siege restart rules, and the siegable
// clan hall keys (ChSiegeClanMinLevel, ChAttackerMaxClans) for the clan
// hall sieges.
func loadSiegeConfig(paths gameServerPaths) (siege.Config, []string, error) {
	props, err := config.LoadFile(paths.SiegeConfigPath)
	if err != nil {
		return siege.Config{}, nil, err
	}
	f := config.NewFields(props, "siege")
	def := siege.DefaultConfig()
	cfg := siege.Config{
		Length:          time.Duration(f.Int("SiegeLength", int(def.Length/time.Minute))) * time.Minute,
		MinClanLevel:    f.Int("SiegeClanMinLevel", def.MinClanLevel),
		MaxAttackers:    f.Int("AttackerMaxClans", def.MaxAttackers),
		MaxDefenders:    f.Int("DefenderMaxClans", def.MaxDefenders),
		AttackerRespawn: time.Duration(f.Int("AttackerRespawn", int(def.AttackerRespawn/time.Millisecond))) * time.Millisecond,
	}
	var notApplied []string
	for _, k := range siegeKeysNotApplied {
		if _, ok := props.Lookup(k.key); ok {
			notApplied = append(notApplied, k.key+" ("+k.issue+")")
		}
	}
	return cfg, notApplied, f.Err()
}

// provideSieges restores the castle sieges' registrations, once the
// castles have their owners, writing through the persistence worker. Each
// siege turns on the siege zone of its castle.
func provideSieges(ctx bootContext, paths gameServerPaths, pool *sql.DB, worker *persist.Worker, data *gameData, clans *clan.Service, castles *castle.Manager, log zerolog.Logger) (*siege.Engine, error) {
	cfg, notApplied, err := loadSiegeConfig(paths)
	if err != nil {
		return nil, err
	}
	if len(notApplied) > 0 {
		log.Warn().Strs("keys", notApplied).Str("file", paths.SiegeConfigPath).
			Msg("siege.properties keys are read but not applied yet")
	}
	var fields []*zone.Siege
	if data.Zones != nil {
		fields = zone.OfKind[*zone.Siege](data.Zones)
	}
	sieges := siege.New(cfg, castles, clans.Table(), fields, gamesql.NewSiegeStore(pool), worker, log)
	if err := sieges.Restore(ctx); err != nil {
		return nil, fmt.Errorf("restore sieges: %w", err)
	}
	return sieges, nil
}

// startSieges runs the siege calendars, telling the world through link,
// before any character can log in; on shutdown it stops them.
func startSieges(lc fx.Lifecycle, sieges *siege.Engine, link *network.GameClientLink, pool *sim.Pool) {
	queue := pool.NewQueue("sieges")
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			sieges.Start(queue, network.SiegeNotifier(link))
			return nil
		},
		OnStop: func(context.Context) error {
			queue.Close()
			return nil
		},
	})
}
