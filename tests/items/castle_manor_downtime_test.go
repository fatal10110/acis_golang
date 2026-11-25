package items

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Owner ruling on #4694: the manor stores when it last rolled its period
// over, and a server down across the refresh time rolls the missed period
// over once as it starts.

// TestCastleManorRollsOverMissedRefreshOnce boots Gludio's manor, its
// running period buying Dark Coda at 50 and Red Coda at 7 and its next one
// selling Dark Coda seeds at 5 and buying Dark Coda at 30, with the time of
// its last rollover stored. A server stopped before the 20:00 refresh and
// started after it, at 20:30, at 02:00 or in the 21:00 window where the due
// change rolls over itself, rolls over once: the next period's lists become
// the running ones, its cost is charged once and the rollover's time is
// stored. A server whose downtime held no refresh time rolls nothing over.
func TestCastleManorRollsOverMissedRefreshOnce(t *testing.T) {
	t.Parallel()
	yesterday := time.Date(2026, 10, 4, 20, 0, 0, 0, time.UTC).UnixMilli()
	today := time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC).UnixMilli()
	for _, tc := range []struct {
		name     string
		stored   int64
		boot     time.Time
		rollover bool
		status   castlemanor.Status
	}{
		{"missed, boot at 20:30", yesterday, time.Date(2026, 10, 5, 20, 30, 0, 0, time.UTC), true, castlemanor.StatusModifiable},
		{"missed, boot at 02:00", yesterday, time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC), true, castlemanor.StatusModifiable},
		{"missed, boot at 21:03", yesterday, time.Date(2026, 10, 5, 21, 3, 0, 0, time.UTC), true, castlemanor.StatusModifiable},
		{"rolled, boot at 20:30", today, time.Date(2026, 10, 5, 20, 30, 0, 0, time.UTC), false, castlemanor.StatusModifiable},
		{"rolled, boot at 21:03", today, time.Date(2026, 10, 5, 21, 3, 0, 0, time.UTC), false, castlemanor.StatusModifiable},
		{"rolled, boot at 02:00", today, time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC), false, castlemanor.StatusModifiable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			manorCfg, _ := shippedManor()
			cfg := castlemanor.DefaultConfig()
			cfg.SavePeriod = 0
			w := bootCastleWorld(t, 1, func(db *sql.DB) {
				for _, q := range []string{
					"UPDATE castle SET treasury = 5000 WHERE id = 1",
					"INSERT INTO castle_manor_procure VALUES (1, 5073, 40, 100, 50, 1, 0), (1, 5068, 9, 10, 7, 2, 0), (1, 5073, 100, 100, 30, 1, 1)",
					"INSERT INTO castle_manor_production VALUES (1, 5016, 10, 10, 5, 1)",
					"INSERT INTO server_memo (var, value) VALUES ('manorLastRollover', '" + strconv.FormatInt(tc.stored, 10) + "')",
				} {
					if _, err := db.ExecContext(context.Background(), q); err != nil {
						t.Fatalf("seed manor: %v", err)
					}
				}
			},
				gameservertest.WithManor(manorCfg),
				gameservertest.WithCastleManor(cfg, tc.boot, castlemanor.WithRoll(func(int) int { return 0 })),
			)
			m := w.srv.CastleManor
			w.srv.CastleManorClock.Advance(0)
			if got := m.Status(); got != tc.status {
				t.Fatalf("status after start = %s, want %s", got, tc.status)
			}
			gludio, _ := w.srv.Castles.Get(1)
			running := m.CropProcure(1, false)
			storedLast := queryCastleInt(t, w.srv, "SELECT CAST(value AS SIGNED) FROM server_memo WHERE var = 'manorLastRollover'")
			if !tc.rollover {
				if len(running) != 2 || gludio.Treasury() != 5000 {
					t.Fatalf("running crops %+v, treasury %d; want the period kept and 5000", running, gludio.Treasury())
				}
				if storedLast != tc.stored {
					t.Fatalf("stored last rollover = %d, want %d kept", storedLast, tc.stored)
				}
				return
			}
			// The rollover charges the next period once: 10 seeds at 5 and
			// 100 crops at 30.
			if want := int64(5000 - 10*5 - 100*30); gludio.Treasury() != want {
				t.Fatalf("treasury after start = %d, want %d (one rollover)", gludio.Treasury(), want)
			}
			if len(running) != 1 || running[0].ID != darkCodaCropID || running[0].Price != 30 || running[0].Amount() != 100 {
				t.Fatalf("running crops = %+v, want the former next period's 100 Dark Coda at 30", running)
			}
			seeds := m.SeedProduction(1, false)
			if len(seeds) != 1 || seeds[0].Price != 5 || seeds[0].Amount() != 10 {
				t.Fatalf("running seeds = %+v, want the former next period's 10 at 5", seeds)
			}
			if got := queryCastleInt(t, w.srv, "SELECT COUNT(*) FROM castle_manor_procure WHERE crop_id = ?", redCodaCropID); got != 0 {
				t.Fatalf("stored Red Coda rows = %d, want the finished period's gone", got)
			}
			if got := queryCastleInt(t, w.srv, "SELECT COUNT(*) FROM castle_manor_procure WHERE castle_id = 1 AND next_period = 0 AND crop_id = ? AND price = 30 AND amount = 100", darkCodaCropID); got != 1 {
				t.Fatalf("stored running Dark Coda rows at 30 = %d, want 1", got)
			}
			if storedLast <= tc.stored || storedLast > tc.boot.UnixMilli() {
				t.Fatalf("stored last rollover = %d, want the start's rollover, past %d", storedLast, tc.stored)
			}
		})
	}
}
