package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// lotteryFixture is what WithLottery asked for.
type lotteryFixture struct {
	cfg  lottery.Config
	seed func(*sql.DB)
	opts []lottery.Option
}

// WithLottery runs the Lucky Lottery with cfg on the fixture database: once
// the characters are seeded, seed (when set) adjusts the games and items
// rows, then the rounds are restored and the lottery started, its calendar
// on the actor queues' clock. Server.Lottery is the lottery. Without it no
// round runs.
func WithLottery(cfg lottery.Config, seed func(db *sql.DB), opts ...lottery.Option) Option {
	return func(o *options) { o.lottery = &lotteryFixture{cfg: cfg, seed: seed, opts: opts} }
}

// newLottery builds the lottery WithLottery asked for, nil without it.
func (f *lotteryFixture) newLottery(db *sql.DB, worker *persist.Worker, state *world.State, q *queues, log zerolog.Logger) *lottery.Lottery {
	if f == nil {
		return nil
	}
	return lottery.New(f.cfg, gamesql.NewLotteryStore(db), worker, network.NewLotteryAnnouncer(state), q.NewQueue("lottery"), log, f.opts...)
}

// start seeds, restores and starts l, stopping it when tb ends.
func (f *lotteryFixture) start(tb testing.TB, db *sql.DB, ids *sequentialIDs, l *lottery.Lottery) {
	tb.Helper()
	if f == nil {
		return
	}
	if f.seed != nil {
		ids.seed(tb, db, func() { f.seed(db) })
	}
	if err := l.Restore(context.Background()); err != nil {
		tb.Fatalf("restore lottery: %v", err)
	}
	l.Start()
	tb.Cleanup(l.Stop)
}
