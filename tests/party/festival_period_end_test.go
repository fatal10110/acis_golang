package party

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFestivalCycleNearThePeriodEnd boots a signed-up party shortly before
// the Seven Signs period ends on Monday 18:00. A cycle that cannot finish
// before the period end is skipped and the next one waits a whole cycle
// (38 minutes); one that can still finish opens its 19-minute signup and
// takes the party into the arena.
func TestFestivalCycleNearThePeriodEnd(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		start time.Time
		next  time.Duration
		runs  bool
	}{
		// The first cycle begins at 17:32, 28 minutes before the period end.
		{"skipped", time.Date(2026, time.October, 26, 17, 30, 0, 0, time.UTC), 38 * time.Minute, false},
		// The first cycle begins at 17:02, 58 minutes before the period end.
		{"runs", time.Date(2026, time.October, 26, 17, 0, 0, 0, time.UTC), 19 * time.Minute, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.start.Weekday() != time.Monday {
				t.Fatalf("start %v is a %v, want a Monday", tc.start, tc.start.Weekday())
			}
			f := bootFestivalWith(t, festivalSetup{
				seats:      []seat{{"Leader", 20}, {"Member", 20}},
				blueStones: 1000,
				minPlayers: 2,
				start:      tc.start,
				extra: []gameservertest.Option{
					// The period change itself never comes due here.
					gameservertest.WithSevenSignsTimer(func(time.Duration, func()) *time.Timer { return nil }),
				},
			})
			periodEnd := time.Date(2026, time.October, 26, 18, 0, 0, 0, time.UTC)
			if got := f.srv.SevenSigns.NextChange(); !got.Equal(periodEnd) {
				t.Fatalf("period end = %v, want %v", got, periodEnd)
			}
			f.invite(t, 0, 1, 0)
			f.command(t, 1, "1")
			f.command(t, 0, "1")
			if got := festivalPage(t, f.command(t, 0, "2 6360")); got != "page 2e" {
				t.Fatalf("payment = %q, want page 2e", got)
			}
			f.quiet(t)

			f.timers.fire(t, 2*time.Minute) // the first cycle comes due
			f.timers.fire(t, tc.next)
			for _, p := range f.players {
				frames := drainFrames(t, p.c)
				_, teleported := firstFrame(frames, serverpackets.OpcodeTeleportToLocation)
				if teleported != tc.runs {
					t.Fatalf("%s teleported = %v, want %v (frames %x)", p.name, teleported, tc.runs, opcodes(frames))
				}
				if got := sayings(frames); len(got) != 0 {
					t.Fatalf("%s heard %q", p.name, got)
				}
			}
			if got := f.srv.Festival.Initialized(); got != tc.runs {
				t.Fatalf("festival initialized = %v, want %v", got, tc.runs)
			}
		})
	}
}
