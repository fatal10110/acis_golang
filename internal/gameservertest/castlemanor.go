package gameservertest

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// castleManorOptions is the manor WithCastleManor asks for.
type castleManorOptions struct {
	cfg   castlemanor.Config
	start time.Time
	opts  []castlemanor.Option
}

// WithCastleManor runs the castles' manor with cfg over the WithCastles
// castles and the WithManor seeds, restored from the database once the
// castles are, its cycle on an inline clock reading start that only
// Server.CastleManorClock moves; without it the server runs no manor.
func WithCastleManor(cfg castlemanor.Config, start time.Time, opts ...castlemanor.Option) Option {
	return func(o *options) { o.castleManor = &castleManorOptions{cfg: cfg, start: start, opts: opts} }
}

// newCastleManor builds the manor WithCastleManor asks for, nil without
// it.
func newCastleManor(db *sql.DB, o *options, castles *castle.Manager, clans *clan.Service, worker *persist.Worker, log zerolog.Logger) *castlemanor.Manager {
	if o.castleManor == nil {
		return nil
	}
	return castlemanor.New(o.castleManor.cfg, o.manor.Seeds, castles, clans.Table(), gamesql.NewManorStore(db), worker, log, o.castleManor.opts...)
}

// startCastleManor restores the manor's stored lists and starts its cycle
// on a fresh inline clock, as the boot path does once the castles have
// their owners. It returns the clock, nil without a manor.
func startCastleManor(t *testing.T, m *castlemanor.Manager, o *options, gcl *network.GameClientLink) *sim.Inline {
	t.Helper()
	if m == nil {
		return nil
	}
	if err := m.Restore(context.Background()); err != nil {
		t.Fatalf("restore manor: %v", err)
	}
	clock := sim.NewInline(o.castleManor.start)
	queue := clock.NewQueue("castle-manor")
	t.Cleanup(queue.Close)
	m.Start(queue, network.CastleManorEffects(gcl))
	return clock
}
