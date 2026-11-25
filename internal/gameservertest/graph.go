package gameservertest

import (
	"database/sql"
	"testing"

	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/gameserver/app"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
)

// BootGraph is the shared provider graph Boot builds its server from, with
// db supplied in place of the production database pool (#4709). The
// production graph divergence test compares it with cmd/gameserver's.
func BootGraph(db *sql.DB) fx.Option {
	return fx.Options(fx.Supply(db), app.Stores)
}

// graphStores are the stores Boot takes from BootGraph.
type graphStores struct {
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
}

// buildGraph builds BootGraph over db. Its providers register no lifecycle
// hooks yet, so the app is never started.
func buildGraph(t *testing.T, db *sql.DB) graphStores {
	t.Helper()
	var stores graphStores
	graph := fx.New(BootGraph(db), fx.NopLogger, fx.Invoke(func(s graphStores) { stores = s }))
	if err := graph.Err(); err != nil {
		t.Fatalf("build the shared provider graph: %v", err)
	}
	return stores
}
