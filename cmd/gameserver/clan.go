package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// loadClanConfig reads the clans.properties clan and alliance penalties,
// dissolution delay, alliance size and clan warehouse withdrawal right the
// clan core uses and the players.properties clan skill item switch,
// defaulting as the reference does when a key is missing.
func loadClanConfig(paths gameServerPaths, _ zerolog.Logger) (clan.Config, error) {
	props, err := config.LoadFile(paths.ClansConfigPath)
	if err != nil {
		return clan.Config{}, err
	}
	players, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return clan.Config{}, err
	}
	f := config.NewFields(props, "clans")
	cfg := clan.Config{
		JoinDays:                        f.Int("DaysBeforeJoinAClan", 5),
		CreateDays:                      f.Int("DaysBeforeCreateAClan", 10),
		MembersForWar:                   f.Int("ClanMembersForWar", 15),
		WarPenaltyDays:                  f.Int("ClanWarPenaltyWhenEnded", 5),
		LifeCrystalNeeded:               players.Bool("LifeCrystalNeeded", true),
		MembersCanWithdrawFromWarehouse: f.Bool("MembersCanWithdrawFromClanWH", false),
		AllyJoinDaysWhenLeft:            f.Int("DaysBeforeJoinAllyWhenLeaved", 1),
		AllyJoinDaysWhenDismissed:       f.Int("DaysBeforeJoinAllyWhenDismissed", 1),
		AcceptClanDaysWhenDismissed:     f.Int("DaysBeforeAcceptNewClanWhenDismissed", 1),
		CreateAllyDaysWhenDissolved:     f.Int("DaysBeforeCreateNewAllyWhenDissolved", 10),
		MaxClansInAlly:                  f.Int("MaxNumOfClansInAlly", 3),
		DissolveDays:                    f.Int("DaysToPassToDissolveAClan", 7),
	}
	return cfg, f.Err()
}

// provideClans restores every clan from the database, after the id factory
// has dropped the clans whose leader no longer exists and the war
// penalties that ran out while the server was down, gives each owning clan
// its clan hall among the loaded halls, clears the crest ids
// whose image the crest cache does not hold and the alliances whose
// leading clan no longer exists, and returns the clan service
// writing through the persistence worker. A stored clan skill whose
// definition is not loaded is skipped.
func provideClans(ctx bootContext, pool *sql.DB, ids *idfactory.Allocator, worker *persist.Worker, cfg clan.Config, crests *datacache.Crests, data *gameData, log zerolog.Logger) (*clan.Service, error) {
	store := gamesql.NewClanStore(pool)
	now := time.Now()
	if err := store.DeleteExpiredWars(ctx, now.UnixMilli()); err != nil {
		return nil, fmt.Errorf("restore clans: %w", err)
	}
	snap, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("restore clans: %w", err)
	}
	snap.KeepSkills(func(sk clan.Skill) bool {
		_, ok := data.Skills.Get(skill.ID(sk.ID), sk.Level)
		return ok
	})
	table := clan.NewTable()
	table.Restore(snap, now, cfg.JoinDays)
	halls, err := store.LoadHallOwners(ctx)
	if err != nil {
		return nil, fmt.Errorf("restore clans: %w", err)
	}
	table.RestoreHalls(halls, func(id int32) bool {
		_, ok := data.ClanHalls.Get(int(id))
		return ok
	})
	log.Info().Int("clans", table.Len()).Msg("clans loaded")
	service := clan.NewService(table, store, worker, ids, cfg, time.Now, log)
	service.DropMissingCrests(crests)
	service.DropDanglingAlliances()
	return service, nil
}

// startClanDissolutions arms the pending clan dissolutions before any
// character can log in, each destroying its clan through link once it comes
// due; on shutdown it stops them.
func startClanDissolutions(lc fx.Lifecycle, clans *clan.Service, link *network.GameClientLink, pool *sim.Pool) {
	queue := pool.NewQueue("clan-dissolution")
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			clans.StartDissolutions(queue, link)
			return nil
		},
		OnStop: func(context.Context) error {
			queue.Close()
			return nil
		},
	})
}
