package gameservertest

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/gameserver/app"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// BootGraph is the shared provider graph Boot builds its server from, with
// db and log supplied in place of the production database pool and logger
// (#4709). The production graph divergence test compares it with
// cmd/gameserver's.
func BootGraph(db *sql.DB, log zerolog.Logger) fx.Option {
	return fx.Options(fx.Supply(db, log), app.Graph)
}

// graphValues are the values Boot takes from BootGraph.
type graphValues struct {
	fx.In

	Characters      *gamesql.CharacterStore
	Items           *gamesql.ItemStore
	Shortcuts       *gamesql.ShortcutStore
	Hennas          *gamesql.HennaStore
	RecipeBooks     *gamesql.RecipeBookStore
	Macros          *gamesql.MacroStore
	Bookmarks       *gamesql.BookmarkStore
	Recommendations *gamesql.RecommendationStore
	Quests          *gamesql.QuestStore
	Memos           *gamesql.MemoStore
	Subclasses      *gamesql.SubclassStore
	Pets            *gamesql.PetStore
	BossZones       *gamesql.BossZoneStore
	Couples         *gamesql.CoupleStore
	GroundItems     *gamesql.GroundItemStore
	Petitions       *gamesql.PetitionStore
	Relations       *gamesql.RelationStore
	Spawns          *gamesql.SpawnStore

	Persist    *persist.Worker
	ItemWrites *persist.Order
	Sessions   *network.SessionValidator
}

// bootGraph is the started BootGraph app and the values Boot takes from it.
type bootGraph struct {
	graphValues

	// stop stops the app, running its providers' stop hooks; it runs once.
	// Boot registers it for cleanup where those hooks sit in the production
	// stop order. A Boot that fails before then still stops the app through
	// the cleanup buildGraph registers.
	stop func()
}

// buildGraph builds and starts BootGraph over db and log.
func buildGraph(t *testing.T, db *sql.DB, log zerolog.Logger) bootGraph {
	t.Helper()
	var values graphValues
	graph := fx.New(BootGraph(db, log), fx.NopLogger, fx.Invoke(func(v graphValues) { values = v }))
	if err := graph.Err(); err != nil {
		t.Fatalf("build the shared provider graph: %v", err)
	}
	if err := graph.Start(context.Background()); err != nil {
		t.Fatalf("start the shared provider graph: %v", err)
	}
	stop := sync.OnceFunc(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
		defer cancel()
		if err := graph.Stop(ctx); err != nil {
			t.Errorf("stop the shared provider graph: %v", err)
		}
	})
	cleanup(t, stop)
	return bootGraph{graphValues: values, stop: stop}
}
