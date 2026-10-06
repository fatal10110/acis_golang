package lifecycle

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedTask runs the task path with its scripts.xml entry as the
// datapack lists it, the task clock starting at start.
func shippedTask(t *testing.T, path string, ctor func() script.Script, start time.Time) []gameservertest.Option {
	t.Helper()
	list, err := gamexml.LoadScriptList(datapack.Path(t, "data", "xml", "scripts.xml"), zerolog.Nop())
	if err != nil {
		t.Fatalf("load scripts.xml: %v", err)
	}
	i := slices.IndexFunc(list, func(l script.Listing) bool { return l.Path == path })
	if i < 0 {
		t.Fatalf("scripts.xml does not list %s", path)
	}
	return []gameservertest.Option{
		gameservertest.WithScripts(list[i:i+1], script.Catalog{path: ctor}),
		gameservertest.WithScheduledTasks(start),
	}
}

// advanceTo moves srv's task clock to at and lets what it started finish.
func advanceTo(t *testing.T, srv *gameservertest.Server, at time.Time) {
	t.Helper()
	srv.ScheduleClock.Advance(at.Sub(srv.ScheduleClock.Now()))
	srv.Settle(t)
	srv.FlushPersistence(t)
}

func queryInt64(t *testing.T, db *sql.DB, query string) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRowContext(context.Background(), query).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// castleColumns reads the money and tax columns of castle id's row.
func castleColumns(t *testing.T, db *sql.DB, id int) [5]int64 {
	t.Helper()
	var c [5]int64
	if err := db.QueryRowContext(context.Background(),
		`SELECT treasury, taxRevenue, seedIncome, currentTaxPercent, nextTaxPercent FROM castle WHERE id = ?`, id,
	).Scan(&c[0], &c[1], &c[2], &c[3], &c[4]); err != nil {
		t.Fatalf("read castle %d: %v", id, err)
	}
	return c
}

// TestCastleTaxRefreshRunsDaily boots at 23:59, after the shipped daily
// 00:00:00 start of the day has passed, so the castle tax refresh first
// runs at the next midnight, not at boot. At midnight, not before, Gludio
// Castle, owned, banks its tax revenue and seed income and puts its next
// rate in force; Dion Castle, free, loses its treasury and has both rates
// back at its default. Both rows are stored.
func TestCastleTaxRefreshRunsDaily(t *testing.T) {
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	boot := time.Date(2026, 10, 5, 23, 59, 0, 0, time.UTC)
	midnight := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	srv := gameservertest.Boot(t, append(shippedTask(t, "task.CastleTaxRefresh", scripttask.CastleTaxRefresh, boot),
		gameservertest.WithCharacter("Lord", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []string{
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
					SELECT 0x7f000071, 'Lords', 5, 1, obj_Id FROM characters WHERE char_name = 'Lord'`,
				`UPDATE characters SET clanid = 0x7f000071 WHERE char_name = 'Lord'`,
				`UPDATE castle SET treasury = 1000, taxRevenue = 300, seedIncome = 200, currentTaxPercent = 10, nextTaxPercent = 20 WHERE id = 1`,
				`UPDATE castle SET treasury = 400, taxRevenue = 50, seedIncome = 60, currentTaxPercent = 5, nextTaxPercent = 7 WHERE id = 2`,
			} {
				if _, err := db.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("seed castles: %v", err)
				}
			}
		}),
	)...)

	advanceTo(t, srv, midnight.Add(-time.Second))
	if got, want := castleColumns(t, srv.DB, 1), [5]int64{1000, 300, 200, 10, 20}; got != want {
		t.Fatalf("Gludio before midnight = %v, want it untouched %v", got, want)
	}
	advanceTo(t, srv, midnight)
	if got, want := castleColumns(t, srv.DB, 1), [5]int64{1500, 0, 0, 20, 20}; got != want {
		t.Errorf("Gludio after the refresh = %v, want %v", got, want)
	}
	dion, ok := castles.Get(2)
	if !ok {
		t.Fatal("castles.xml has no castle 2")
	}
	rate := int64(dion.Tax.Rate)
	if got, want := castleColumns(t, srv.DB, 2), [5]int64{0, 0, 0, rate, rate}; got != want {
		t.Errorf("Dion after the refresh = %v, want %v", got, want)
	}
}

// TestSevenSignsUpdateRunsHourly runs the hourly Seven Signs save. Its
// rows are changed behind the server's back after boot; at the next full
// hour, not before, the status row is written back from memory in every
// period, and the festival scores too, except in the seal validation
// period.
func TestSevenSignsUpdateRunsHourly(t *testing.T) {
	for _, tc := range []struct {
		name          string
		period        sevensigns.Period
		wantFestival  int64
		wantPeriodCol string
	}{
		{"competition", sevensigns.Competition, 1, "COMPETITION"},
		{"seal validation", sevensigns.SealValidation, 0, "SEAL_VALIDATION"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			boot := time.Date(2026, 10, 5, 12, 59, 30, 0, time.UTC)
			hour := time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC)
			srv := gameservertest.Boot(t, append(shippedTask(t, "task.SevenSignsUpdate", scripttask.SevenSignsUpdate, boot),
				gameservertest.WithCharacter("Newbie", 1, 0),
				gameservertest.WithWantChars(1),
				seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) { row.Period = tc.period }),
				gameservertest.WithFestivalSeed(func(store *gamesql.FestivalStore) {
					if err := store.SaveScores(context.Background(), []festival.Score{
						{FestivalID: 2, Cabal: sevensigns.Dawn, Cycle: 1, Date: 1767225600000, Score: 42, Members: "Ann"},
					}); err != nil {
						t.Fatalf("seed festival scores: %v", err)
					}
				}),
			)...)
			for _, q := range []string{
				`UPDATE seven_signs_status SET active_period = 'RESULTS' WHERE id = 0`,
				`DELETE FROM seven_signs_festival`,
			} {
				if _, err := srv.DB.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}

			advanceTo(t, srv, hour.Add(-time.Second))
			if _, period, _ := statusRow(t, srv.DB); period != "RESULTS" {
				t.Fatalf("status before the hour = %s, want it untouched", period)
			}
			advanceTo(t, srv, hour)
			if _, period, _ := statusRow(t, srv.DB); period != tc.wantPeriodCol {
				t.Errorf("status after the save = %s, want %s", period, tc.wantPeriodCol)
			}
			if got := queryInt64(t, srv.DB, `SELECT COUNT(*) FROM seven_signs_festival WHERE festivalId = 2 AND cabal = 'DAWN' AND cycle = 1 AND score = 42`); got != tc.wantFestival {
				t.Errorf("seeded festival score rows after the save = %d, want %d", got, tc.wantFestival)
			}
		})
	}
}
