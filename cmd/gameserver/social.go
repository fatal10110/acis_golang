package main

import (
	"context"
	"database/sql"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// provideRelations loads the friend and block lists. It takes the id
// allocator only to run after it: the allocator's boot cleanup drops the
// relation rows of characters that no longer exist.
func provideRelations(ctx bootContext, pool *sql.DB, _ *idfactory.Allocator, log zerolog.Logger) (*relation.Manager, *gamesql.RelationStore, error) {
	store := gamesql.NewRelationStore(pool)
	rows, err := store.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	log.Info().Int("relations", len(rows)).Msg("character relations loaded")
	return relation.NewManager(rows), store, nil
}

// startRelationPersistence writes the friend and block lists back at
// shutdown, after the game listener has stopped and every player has left.
func startRelationPersistence(lc fx.Lifecycle, relations *relation.Manager, store *gamesql.RelationStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, shutdownSaveTimeout, log, "save character relations", func(ctx context.Context) error {
				return store.Save(ctx, relations.Rows())
			})
			return nil
		},
	})
}
