package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// defaultHeroMinMatches is the number of Olympiad matches a noble needs to
// be elected hero when events.properties does not set one.
const defaultHeroMinMatches = 5

// loadHeroMinMatches reads OlyMinMatchesToBeClassed from events.properties:
// the matches a noble needs to be elected hero.
func loadHeroMinMatches(paths gameServerPaths) (int, error) {
	props, err := config.LoadFile(paths.EventsConfigPath)
	if err != nil {
		return 0, err
	}
	f := config.NewFields(props, "events")
	n := f.Int("OlyMinMatchesToBeClassed", defaultHeroMinMatches)
	return n, f.Err()
}

// provideHeroes returns the heroes, persisting through the gameserver
// database on worker's lanes, showing the clans clans holds and naming the
// raid bosses and castles of their diaries from data and castles.
func provideHeroes(paths gameServerPaths, db *sql.DB, clans *clan.Service, data *gameData, castles *castle.Manager, worker *persist.Worker, log zerolog.Logger) (*hero.Manager, error) {
	minMatches, err := loadHeroMinMatches(paths)
	if err != nil {
		return nil, err
	}
	names := hero.TableNames{NPCs: data.NPCs, Castles: castles}
	return hero.New(gamesql.NewHeroStore(db), clans.Table(), names, worker, minMatches, time.Now, log), nil
}

// startHeroes restores the heroes before any character can log in and lets
// an election reach the heroes online through link. At stop the heroes'
// messages are queued for storing; the persistence worker, stopped after
// the game server, lands that and whatever else is still queued.
func startHeroes(lc fx.Lifecycle, heroes *hero.Manager, link *network.GameClientLink) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := heroes.Restore(ctx); err != nil {
				return err
			}
			heroes.Start(link)
			return nil
		},
		OnStop: func(context.Context) error {
			heroes.Shutdown()
			return nil
		},
	})
}
