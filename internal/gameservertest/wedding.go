package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// WithWeddingConfig replaces the shipped wedding settings.
func WithWeddingConfig(cfg wedding.Config) Option {
	return func(o *options) { o.weddingConfig = &cfg }
}

// bootWedding loads the stored couples as the boot path does.
func bootWedding(t *testing.T, db *sql.DB, ids wedding.IDs, cfg *wedding.Config) (*wedding.Manager, *gamesql.CoupleStore) {
	t.Helper()
	store := gamesql.NewCoupleStore(db)
	couples, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load couples: %v", err)
	}
	config := wedding.DefaultConfig()
	if cfg != nil {
		config = *cfg
	}
	return wedding.NewManager(config, ids, couples), store
}

// SaveCouples writes every couple to mods_wedding, as the shutdown save
// does.
func (s *Server) SaveCouples(tb testing.TB) {
	tb.Helper()
	if err := s.coupleRows.Save(context.Background(), s.Couples.Couples()); err != nil {
		tb.Fatalf("save couples: %v", err)
	}
}
