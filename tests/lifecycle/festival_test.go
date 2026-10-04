package lifecycle

import (
	"bytes"
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The record's festival page lists, for each of the five festivals
// numbered from 1, what it is worth (60, 70, 100, 120, 150) and the dusk
// then dawn best score of the current cycle with the party that set it.
// A blank score names one empty member; a better score from another cycle
// is not shown.
//
// Seeded on top of cycle 1's blank scores: dawn festival 0 scored 7 by
// Ann and Bob, dusk festival 4 scored 150 by Cid, and dusk festival 0
// scored 99 in cycle 0.
func TestRecordFestivalPageListsTheCycleBestScores(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithFestivalSeed(func(store *gamesql.FestivalStore) {
			if err := store.SaveScores(context.Background(), []festival.Score{
				{FestivalID: 0, Cabal: sevensigns.Dawn, Cycle: 1, Date: 1767225600000, Score: 7, Members: "Ann,Bob"},
				{FestivalID: 4, Cabal: sevensigns.Dusk, Cycle: 1, Date: 1767225600000, Score: 150, Members: "Cid"},
				{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 0, Date: 1767225600000, Score: 99, Members: "Old"},
			}); err != nil {
				t.Fatalf("seed festival scores: %v", err)
			}
		}),
	)
	c := srv.Client
	startInWorld(t, c)

	blank := []byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00} // score 0, one empty name
	want := []byte{0xf5, 0x02, 0x01, 0x01, 0x00, 0x05}
	want = append(want, 0x01, 0x3c, 0x00, 0x00, 0x00)
	want = append(want, blank...)
	want = append(want, 0x07, 0x00, 0x00, 0x00, 0x02, 'A', 0, 'n', 0, 'n', 0, 0, 0, 'B', 0, 'o', 0, 'b', 0, 0, 0)
	for _, f := range []struct{ number, worth byte }{{2, 0x46}, {3, 0x64}, {4, 0x78}} {
		want = append(want, f.number, f.worth, 0x00, 0x00, 0x00)
		want = append(want, blank...)
		want = append(want, blank...)
	}
	want = append(want, 0x05, 0x96, 0x00, 0x00, 0x00)
	want = append(want, 0x96, 0x00, 0x00, 0x00, 0x01, 'C', 0, 'i', 0, 'd', 0, 0, 0)
	want = append(want, blank...)

	c.Send(encodeRequestSSQStatus(2))
	if got := c.Read(); !bytes.Equal(got, want) {
		t.Fatalf("festival page = % x\nwant            % x", got, want)
	}
	assertQuietClient(t, c, "festival page")
}

// changerOver returns a Seven Signs state and festival over srv's database,
// wired as the production boot wires them, whose period change fires on
// demand; the festival's own timers never fire.
func changerOver(t *testing.T, srv *gameservertest.Server) (state *sevensigns.State, fest *festival.Manager, fire func()) {
	t.Helper()
	ctx := context.Background()
	var change func()
	state = sevensigns.NewState(gamesql.NewSevenSignsStore(srv.DB), network.NewSevenSignsBroadcaster(srv.State), zerolog.Nop(), time.Now,
		func(_ time.Duration, fn func()) *time.Timer { change = fn; return nil })
	if err := state.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	fest = festival.New(festival.DefaultConfig(), gamesql.NewFestivalStore(srv.DB), state, zerolog.Nop(), time.Now,
		func(time.Duration, func()) *time.Timer { return nil })
	if err := fest.Restore(ctx, state.CurrentCycle()); err != nil {
		t.Fatalf("festival Restore: %v", err)
	}
	state.SetFestival(fest)
	fest.Start()
	state.Start()
	return state, fest, func() { change() }
}

// Recruiting's end starts a festival schedule, counting a festival cycle,
// and the period change's save writes it into the status row. A server
// started during recruiting has counted one already: the seeded 1 becomes
// 3.
func TestRecruitingEndCountsAFestivalCycle(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) { row.Period = sevensigns.Recruiting }),
	)
	_, _, fire := changerOver(t, srv)
	fire()

	var period string
	var festivalCycle int
	if err := srv.DB.QueryRow(`SELECT active_period, festival_cycle FROM seven_signs_status WHERE id = 0`).Scan(&period, &festivalCycle); err != nil {
		t.Fatal(err)
	}
	if period != "COMPETITION" || festivalCycle != 3 {
		t.Fatalf("saved status = (%s, festival cycle %d), want (COMPETITION, 3)", period, festivalCycle)
	}
}

// Seal validation's end resets the festival for the new cycle: a blank
// score for each festival and cabal of cycle 2 is saved, and the status
// row's festival cycle and accumulated bonuses go back to zero. The
// earlier cycle's scores stay.
func TestSealValidationEndResetsTheFestival(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) { row.Period = sevensigns.SealValidation }),
		gameservertest.WithFestivalSeed(func(store *gamesql.FestivalStore) {
			ctx := context.Background()
			if err := store.SaveStatus(ctx, festival.Status{FestivalCycle: 3, Bonuses: [5]int{0, 0, 50, 0, 8}}); err != nil {
				t.Fatalf("seed festival status: %v", err)
			}
			if err := store.SaveScores(ctx, []festival.Score{{FestivalID: 1, Cabal: sevensigns.Dawn, Cycle: 1, Score: 12, Members: "Ann"}}); err != nil {
				t.Fatalf("seed festival scores: %v", err)
			}
		}),
	)
	state, fest, fire := changerOver(t, srv)
	fire()
	if state.CurrentCycle() != 2 || state.CurrentPeriod() != sevensigns.Recruiting {
		t.Fatalf("after the change = (cycle %d, %v)", state.CurrentCycle(), state.CurrentPeriod())
	}

	var rows, dawn, nonBlank int
	if err := srv.DB.QueryRow(`SELECT COUNT(*), SUM(cabal = 'DAWN'), SUM(score <> 0 OR members <> '' OR date <> 0)
		FROM seven_signs_festival WHERE cycle = 2`).Scan(&rows, &dawn, &nonBlank); err != nil {
		t.Fatal(err)
	}
	if rows != 10 || dawn != 5 || nonBlank != 0 {
		t.Fatalf("cycle 2 scores = %d rows, %d dawn, %d not blank; want 10, 5, 0", rows, dawn, nonBlank)
	}
	var kept int
	if err := srv.DB.QueryRow(`SELECT score FROM seven_signs_festival WHERE cycle = 1 AND festivalId = 1 AND cabal = 'DAWN'`).Scan(&kept); err != nil || kept != 12 {
		t.Fatalf("cycle 1 score = %d (%v), want 12", kept, err)
	}
	var cycle, festivalCycle, bonus2, bonus4 int
	if err := srv.DB.QueryRow(`SELECT current_cycle, festival_cycle, accumulated_bonus2, accumulated_bonus4 FROM seven_signs_status WHERE id = 0`).
		Scan(&cycle, &festivalCycle, &bonus2, &bonus4); err != nil {
		t.Fatal(err)
	}
	if cycle != 2 || festivalCycle != 0 || bonus2 != 0 || bonus4 != 0 {
		t.Fatalf("saved status = cycle %d, festival cycle %d, bonuses %d/%d; want 2, 0, 0/0", cycle, festivalCycle, bonus2, bonus4)
	}
	if s, ok := fest.HighestScore(sevensigns.Dawn, 1); !ok || s != (festival.Score{FestivalID: 1, Cabal: sevensigns.Dawn, Cycle: 2}) {
		t.Fatalf("dawn festival 1 best score = %+v (%v), want cycle 2's blank", s, ok)
	}
}
