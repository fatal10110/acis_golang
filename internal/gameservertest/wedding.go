package gameservertest

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// WithWeddingConfig replaces the shipped wedding settings.
func WithWeddingConfig(cfg wedding.Config) Option {
	return func(o *options) { o.weddingConfig = &cfg }
}

// bootWedding loads the couples stored in store as the boot path does,
// timing marriage requests out against now (nil means time.Now).
func bootWedding(t *testing.T, store *gamesql.CoupleStore, ids wedding.IDs, cfg *wedding.Config, now func() time.Time) *wedding.Manager {
	t.Helper()
	couples, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load couples: %v", err)
	}
	config := wedding.DefaultConfig()
	if cfg != nil {
		config = *cfg
	}
	return wedding.NewManager(config, ids, couples, now)
}

// SaveCouples writes every couple to mods_wedding, as the shutdown save
// does.
func (s *Server) SaveCouples(tb testing.TB) {
	tb.Helper()
	if err := s.coupleRows.Save(context.Background(), s.Couples.Couples()); err != nil {
		tb.Fatalf("save couples: %v", err)
	}
}
