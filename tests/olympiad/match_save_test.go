package olympiad

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// TestOlympiadMatchRecordsSavedAtTheResult pins that a judged match queues
// both competitors' records (#4696): once persistence drains, with the
// window still open and no stop save, the stored records hold the match's
// points and counts, agreeing with its fight row, and an Olympiad booted
// from that database reads them back.
func TestOlympiadMatchRecordsSavedAtTheResult(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) {
		cfg.BattleMillis = 10_000
		cfg.AnnounceGames = false
	})
	a, b := m.ones[0], m.ones[1]
	m.drawn(t)
	m.fighting(t)
	m.over(t)
	m.srv.FlushPersistence(t)
	if got := m.srv.Olympiad.Period(); got != olympiad.Competition {
		t.Fatalf("period after the match = %v, want the window still open", got)
	}

	if got := matchFights(t, m); len(got) != 1 || got[0].winner != 0 {
		t.Fatalf("olympiad_fights = %+v, want one tie", got)
	}
	ctx := context.Background()
	for _, p := range []*competitor{a, b} {
		var points, done, won, lost, drawn int
		if err := m.srv.DB.QueryRowContext(ctx,
			"SELECT olympiad_points, competitions_done, competitions_won, competitions_lost, competitions_drawn FROM olympiad_nobles WHERE char_id = ?", p.id,
		).Scan(&points, &done, &won, &lost, &drawn); err != nil {
			t.Fatalf("stored record of %s: %v", p.name, err)
		}
		if points != 15 || done != 1 || won != 0 || lost != 0 || drawn != 0 {
			t.Errorf("stored record of %s = %d points, %d/%d/%d/%d; want 15 points and one match done", p.name, points, done, won, lost, drawn)
		}
	}

	// A boot from the same database, without the window's close or the
	// stop save, reads the records the match left.
	loop := sim.NewInline(time.Now())
	again := olympiad.New(olympiad.DefaultConfig(), gamesql.NewOlympiadStore(m.srv.DB), nil, discard{}, nil, loop.NewQueue("olympiad"), zerolog.Nop())
	if err := again.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*competitor{a, b} {
		if n, ok := again.Noble(p.id); !ok || n.Points != 15 || n.CompDone != 1 {
			t.Errorf("restored record of %s = %+v, %v; want 15 points and one match done", p.name, n, ok)
		}
	}
}
