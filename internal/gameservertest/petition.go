package gameservertest

import (
	"context"
	"database/sql"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
)

// WithPetitionConfig replaces the shipped petition settings.
func WithPetitionConfig(cfg petition.Config) Option {
	return func(o *options) { o.petitionConfig = &cfg }
}

// ReleaseID takes nothing back: the sequence never hands an id out twice.
func (s *sequentialIDs) ReleaseID(int32) {}

// bootPetitions loads the stored petitions as the boot path does.
func bootPetitions(t *testing.T, db *sql.DB, chars *gamesql.CharacterStore, ids petition.IDs, cfg *petition.Config) (*petition.Manager, *gamesql.PetitionStore) {
	t.Helper()
	store := gamesql.NewPetitionStore(db)
	records, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load petitions: %v", err)
	}
	names, err := chars.Names(context.Background(), petition.People(records))
	if err != nil {
		t.Fatalf("petition names: %v", err)
	}
	config := petition.DefaultConfig()
	if cfg != nil {
		config = *cfg
	}
	return petition.NewManager(config, ids, time.Now, records, names), store
}

// SavePetitions writes every petition to the petition tables, as the
// shutdown save does.
func (s *Server) SavePetitions(tb testing.TB) {
	tb.Helper()
	if err := s.petitionRows.Save(context.Background(), s.Petitions.Records()); err != nil {
		tb.Fatalf("save petitions: %v", err)
	}
}
