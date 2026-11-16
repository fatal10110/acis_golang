package task

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
)

// bubbleWall returns a wall clock that reads base when called first and then
// moves with the synctest bubble's fake time.
func bubbleWall(base time.Time) func() time.Time {
	origin := time.Now()
	return func() time.Time { return base.Add(time.Since(origin)) }
}

// The in-game time at Start, and at every tick after it, is the one wall
// time gives at that instant, however long boot took between construction
// and Start.
func TestGameClockStartAlignsToWallTimeAfterBoot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Constructed at 00:59:40 local: in-game 05:58, still night.
		now := bubbleWall(time.Date(2026, 7, 10, 0, 59, 40, 0, time.Local))
		c := NewGameClock(now)
		if got := c.Minutes(); got != 358 {
			t.Fatalf("minutes at construction = %d, want 358", got)
		}
		var dayNight []bool
		c.OnDayNight(func(night bool) { dayNight = append(dayNight, night) })

		time.Sleep(25 * time.Second) // boot: 01:00:05 local when the ticker starts
		ticker := c.Start(zerolog.Nop())
		defer ticker.StopAndWait()
		if got := c.Minutes(); got != 360 {
			t.Fatalf("minutes at Start = %d, want 360 (wall time 01:00:05 / 10 s)", got)
		}
		if c.IsNight() {
			t.Fatal("IsNight() at Start = true, want false: 06:00 in-game is day")
		}

		time.Sleep(GameMinute + time.Second)
		synctest.Wait()
		if got := c.Minutes(); got != 361 {
			t.Fatalf("minutes one GameMinute after Start = %d, want 361", got)
		}
		if len(dayNight) != 0 {
			t.Fatalf("day/night listeners ran %v, want none: the boot realignment crosses no boundary on a tick", dayNight)
		}
	})
}

// A tick stalled for several game minutes does not cost the clock those
// minutes: the owed ticks run once the stall ends, the clock again reads
// wall time, and day breaks at the wall instant it is due.
func TestGameClockCatchesUpAfterAStall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := time.Date(2026, 7, 10, 0, 58, 0, 0, time.Local) // in-game 05:48
		now := bubbleWall(base)
		c := NewGameClock(now)

		var mu sync.Mutex
		var dayAt []time.Time
		c.OnDayNight(func(night bool) {
			if !night {
				mu.Lock()
				dayAt = append(dayAt, now())
				mu.Unlock()
			}
		})
		stalled := false
		c.OnMinute(func() {
			if !stalled {
				stalled = true
				time.Sleep(5 * GameMinute) // the first tick holds the ticker for five game minutes
			}
		})

		ticker := c.Start(zerolog.Nop())
		defer ticker.StopAndWait()

		time.Sleep(3*time.Minute + 5*time.Second) // 01:01:05 local
		synctest.Wait()
		if got := c.Minutes(); got != 366 {
			t.Fatalf("minutes at wall 01:01:05 = %d, want 366: every stalled minute is caught up", got)
		}
		mu.Lock()
		defer mu.Unlock()
		want := time.Date(2026, 7, 10, 1, 0, 0, 0, time.Local)
		if len(dayAt) != 1 || !dayAt[0].Equal(want) {
			t.Fatalf("day broke at %v, want once at %v (in-game 06:00)", dayAt, want)
		}
	})
}
