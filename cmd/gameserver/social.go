package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
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

// startRelationPersistence writes the friend and block list changes made
// during the run back at shutdown, after the game listener has stopped and
// every player has left.
func startRelationPersistence(lc fx.Lifecycle, relations *relation.Manager, store *gamesql.RelationStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, shutdownSaveTimeout, log, "save character relations", func(ctx context.Context) error {
				return store.Save(ctx, relations.Changes())
			})
			return nil
		},
	})
}

// providePetitions loads the petitions and the names of their petitioners
// and responders. It takes the id allocator to number new petitions; the
// allocator's boot cleanup has dropped the petitions of characters that no
// longer exist. A row it cannot read stops the load and is logged: the
// petitions read before it are kept.
func providePetitions(ctx bootContext, pool *sql.DB, ids *idfactory.Allocator, characters *gamesql.CharacterStore, gameplay gameplayConfig, log zerolog.Logger) (*petition.Manager, *gamesql.PetitionStore, error) {
	store := gamesql.NewPetitionStore(pool)
	records, err := store.Load(ctx)
	if err != nil {
		log.Error().Err(err).Msg("couldn't load petitions")
	}
	names, err := characters.Names(ctx, petition.People(records))
	if err != nil {
		return nil, nil, err
	}
	log.Info().Int("petitions", len(records)).Msg("petitions loaded")
	return petition.NewManager(gameplay.Petition, ids, time.Now, records, names), store, nil
}

// startPetitionPersistence writes every petition back at shutdown, after
// the game listener has stopped and every player has left.
func startPetitionPersistence(lc fx.Lifecycle, petitions *petition.Manager, store *gamesql.PetitionStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, shutdownSaveTimeout, log, "save petitions", func(ctx context.Context) error {
				return store.Save(ctx, petitions.Records())
			})
			return nil
		},
	})
}
