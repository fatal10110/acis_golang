package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/config"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// loadLotteryConfig reads the server.properties AllowLottery switch and the
// events.properties Lottery* settings, each defaulting to its shipped value
// when a key is missing. A malformed value fails boot.
func loadLotteryConfig(paths gameServerPaths) (lottery.Config, error) {
	serverProps, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return lottery.Config{}, err
	}
	events, err := config.LoadFile(paths.EventsConfigPath)
	if err != nil {
		return lottery.Config{}, err
	}
	def := lottery.DefaultConfig()
	s := config.NewFields(serverProps, "server")
	f := config.NewFields(events, "events")
	cfg := lottery.Config{
		Enabled:              s.Bool("AllowLottery", def.Enabled),
		Prize:                int32Setting(f, "LotteryPrize", def.Prize),
		TicketPrice:          int32Setting(f, "LotteryTicketPrice", def.TicketPrice),
		FiveNumberRate:       f.Float64("Lottery5NumberRate", def.FiveNumberRate),
		FourNumberRate:       f.Float64("Lottery4NumberRate", def.FourNumberRate),
		ThreeNumberRate:      f.Float64("Lottery3NumberRate", def.ThreeNumberRate),
		TwoAndOneNumberPrize: int32Setting(f, "Lottery2and1NumberPrize", def.TwoAndOneNumberPrize),
	}
	if err := s.Err(); err != nil {
		return lottery.Config{}, err
	}
	return cfg, f.Err()
}

// int32Setting reads key as an int, def when missing; a value outside the
// int32 range fails f.
func int32Setting(f *config.Fields, key string, def int32) int32 {
	v := f.Int(key, int(def))
	if v < math.MinInt32 || v > math.MaxInt32 {
		f.Fail(fmt.Errorf("%s %d outside int32 range", key, v))
		return def
	}
	return int32(v)
}

// provideLottery returns the Lucky Lottery, persisting through the
// gameserver database on worker's lanes, announcing to every player in
// state and running its calendar on pool.
func provideLottery(paths gameServerPaths, db *sql.DB, pool *sim.Pool, worker *persist.Worker, state *world.State, log zerolog.Logger) (*lottery.Lottery, error) {
	cfg, err := loadLotteryConfig(paths)
	if err != nil {
		return nil, err
	}
	return lottery.New(cfg, gamesql.NewLotteryStore(db), worker, network.NewLotteryAnnouncer(state), pool.NewQueue("lottery"), log), nil
}

// startLottery restores the stored rounds before any character can log in
// and resumes or starts a round; on shutdown it stops the calendar.
func startLottery(lc fx.Lifecycle, l *lottery.Lottery) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := l.Restore(ctx); err != nil {
				return err
			}
			l.Start()
			return nil
		},
		OnStop: func(context.Context) error {
			l.Stop()
			return nil
		},
	})
}
