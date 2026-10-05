package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// fishChampFixture is what WithFishingChampionship asked for.
type fishChampFixture struct {
	cfg  fishchamp.Config
	seed func(*sql.DB)
	opts []fishchamp.Option
}

// WithFishingChampionship runs the fishing championship with cfg on the
// fixture database: once the characters are seeded, seed (when set)
// adjusts the server_memo and fishing_championship rows, then the
// championship is restored and started, its calendar on the actor queues'
// clock. Server.FishingChampionship is the championship. Without it none
// runs.
func WithFishingChampionship(cfg fishchamp.Config, seed func(db *sql.DB), opts ...fishchamp.Option) Option {
	return func(o *options) { o.fishChamp = &fishChampFixture{cfg: cfg, seed: seed, opts: opts} }
}

// newChampionship builds the championship WithFishingChampionship asked
// for, nil without it.
func (f *fishChampFixture) newChampionship(db *sql.DB, worker *persist.Worker, q *queues, log zerolog.Logger) *fishchamp.Championship {
	if f == nil {
		return nil
	}
	return fishchamp.New(f.cfg, gamesql.NewFishingChampionshipStore(db), worker, q.NewQueue("fishchamp"), log, f.opts...)
}

// start seeds, restores and starts c, stopping it when tb ends.
func (f *fishChampFixture) start(tb testing.TB, db *sql.DB, ids *sequentialIDs, c *fishchamp.Championship) {
	tb.Helper()
	if f == nil {
		return
	}
	if f.seed != nil {
		ids.seed(tb, db, func() { f.seed(db) })
	}
	if err := c.Restore(context.Background()); err != nil {
		tb.Fatalf("restore fishing championship: %v", err)
	}
	c.Start()
	tb.Cleanup(func() { _ = c.Stop(context.Background()) })
}
