package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// provideClanHallFunctions restores the functions each owned clan hall
// rents, once the clans own their halls, writing through the persistence
// worker.
func provideClanHallFunctions(ctx bootContext, pool *sql.DB, worker *persist.Worker, data *gameData, clans *clan.Service, log zerolog.Logger) (*clanhall.Functions, error) {
	fns := clanhall.New(data.ClanHalls, data.ClanHallDecos, clans.Table(), gamesql.NewClanHallFunctionStore(pool), worker, log)
	if err := fns.Restore(ctx); err != nil {
		return nil, fmt.Errorf("restore clan hall functions: %w", err)
	}
	return fns, nil
}

// startClanHallFunctions charges the clan hall function fees from the
// clan warehouses through link before any character can log in; on
// shutdown it stops them.
func startClanHallFunctions(lc fx.Lifecycle, fns *clanhall.Functions, link *network.GameClientLink, pool *sim.Pool) {
	queue := pool.NewQueue("clanhall-functions")
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			fns.Start(queue, link)
			return nil
		},
		OnStop: func(context.Context) error {
			queue.Close()
			return nil
		},
	})
}
