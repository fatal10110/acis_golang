package main

import (
	"database/sql"

	"github.com/fatal10110/acis_golang/internal/gameserver/cursedweapon"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// provideCursedWeapons returns the cursed weapons' lifecycle, persisting
// through the gameserver database on worker's lanes, or nil when cursed
// weapons are disabled.
func provideCursedWeapons(db *sql.DB, data *gameData, worker *persist.Worker, log zerolog.Logger) *cursedweapon.Manager {
	if data.CursedWeapons == nil {
		return nil
	}
	return cursedweapon.New(data.CursedWeapons, gamesql.NewCursedWeaponStore(db), worker, log, nil)
}

// startCursedWeapons restores the held cursed weapons before any character
// can log in, then runs their timers. Their writes need no stop step: the
// persistence worker, stopped after the game server, lands whatever is
// still queued.
func startCursedWeapons(lc fx.Lifecycle, weapons *cursedweapon.Manager, link *network.GameClientLink, log zerolog.Logger) {
	if weapons == nil {
		return
	}
	lc.Append(fx.Hook{OnStart: weapons.Restore})
	startTicker(lc, log, link.StartCursedWeapons)
}
