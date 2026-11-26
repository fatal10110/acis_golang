// Package app is the game server's shared fx provider graph. cmd/gameserver
// composes the production server from it, and the test harness boots from
// the same options, overriding only its seams (the database, the logger, the
// clock, the listener and the fault injectors), so a provider added here
// reaches both with one edit (#4709).
//
// The graph moves here a slice at a time. The providers not moved yet stay
// in cmd/gameserver, where the graph divergence test lists them.
package app

import (
	"go.uber.org/fx"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// Graph is the shared provider graph. It needs the *sql.DB the stores share
// and the zerolog.Logger the persistence worker logs to.
var Graph = fx.Options(stores, persistence, sessions)

// stores provides the persistence stores over the game database.
var stores = fx.Provide(
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
	gamesql.NewCoupleStore,
	gamesql.NewGroundItemStore,
	gamesql.NewPetitionStore,
	gamesql.NewRelationStore,
	gamesql.NewSpawnStore,
)

// persistence provides the worker that runs player, pet and item writes, and
// the ordering both writers of the items table share: the handlers'
// single-row writes and the lazy persistence task write the same rows from
// different lanes, and a row's last write has to be the last one produced
// for it.
var persistence = fx.Provide(NewPersistWorker, persist.NewOrder)

// sessions provides the table of login sessions awaiting the login server's
// answer.
var sessions = fx.Provide(network.NewSessionValidator)
