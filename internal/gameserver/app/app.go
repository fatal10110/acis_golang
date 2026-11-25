// Package app is the game server's shared fx provider graph. cmd/gameserver
// composes the production server from it, and the test harness boots from
// the same options, overriding only its seams (the database, the clock, the
// listener and the fault injectors), so a provider added here reaches both
// with one edit (#4709).
//
// The graph moves here a slice at a time. The providers not moved yet stay
// in cmd/gameserver, where the graph divergence test lists them.
package app

import (
	"go.uber.org/fx"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
)

// Stores provides the character persistence stores over the game database;
// the graph must hold the *sql.DB they share.
var Stores = fx.Provide(
	gamesql.NewCharacterStore,
	gamesql.NewItemStore,
	gamesql.NewShortcutStore,
	gamesql.NewHennaStore,
	gamesql.NewRecipeBookStore,
	gamesql.NewMacroStore,
	gamesql.NewBookmarkStore,
	gamesql.NewRecommendationStore,
	gamesql.NewQuestStore,
	gamesql.NewMemoStore,
	gamesql.NewSubclassStore,
	gamesql.NewPetStore,
	gamesql.NewBossZoneStore,
)
