package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// sweep mirrors the live-effect sweep interval of the task package, which
// cannot be imported here (it depends on this package).
const sweep = time.Second

// Driven by a 1s sweep that runs 700ms off the effect's own phase, every
// tick lands at the first sweep at or after its due instant, less than one
// sweep late; the schedule stays anchored to the start, so the lateness never
// accumulates, and the effect runs exactly its count of ticks.
func TestClaimActionUnderSweepHasBoundedLatenessNoDriftExactCount(t *testing.T) {
	const count, periodSec = 5, 3
	period := periodSec * time.Second
	t0 := time.Unix(1000, 0)
	e := &Effect{Template: modelskill.EffectTemplate{Count: count, Time: periodSec}}
	e.startSchedule(t0)

	ticks := 0
	removed := false
	for k := 0; k < 3*count*periodSec; k++ {
		now := t0.Add(time.Duration(k)*sweep + 700*time.Millisecond)
		due := e.dueAt()
		run, remove := e.claimAction(now)
		if removed {
			if run || remove {
				t.Fatalf("sweep %d after removal = run %v remove %v, want nothing", k, run, remove)
			}
			continue
		}
		if !run {
			if !now.Before(due) {
				t.Fatalf("sweep %d at %v skipped a tick due at %v", k, now.Sub(t0), due.Sub(t0))
			}
			continue
		}
		ticks++
		if want := t0.Add(time.Duration(ticks) * period); !due.Equal(want) {
			t.Fatalf("tick %d was due at %v, want %v: the schedule drifted", ticks, due.Sub(t0), want.Sub(t0))
		}
		if late := now.Sub(due); late < 0 || late >= sweep {
			t.Fatalf("tick %d ran %v after its due instant, want [0, %v)", ticks, late, sweep)
		}
		if remove != (ticks == count) {
			t.Fatalf("tick %d remove = %v, want %v", ticks, remove, ticks == count)
		}
		if remove {
			removed = true
			if !e.dueAt().IsZero() {
				t.Fatalf("dueAt after the last tick = %v, want zero", e.dueAt().Sub(t0))
			}
			continue
		}
		if want := t0.Add(time.Duration(ticks+1) * period); !e.dueAt().Equal(want) {
			t.Fatalf("after tick %d dueAt = %v, want %v: the next tick re-anchored on the late claim", ticks, e.dueAt().Sub(t0), want.Sub(t0))
		}
	}
	if ticks != count {
		t.Fatalf("ran %d ticks, want exactly %d", ticks, count)
	}
	if !removed {
		t.Fatal("the last tick never reported remove")
	}
}

// A claim more than one period late runs a single tick and keeps the next
// one on the original grid, so the following sweep catches up instead of the
// schedule sliding back by the delay.
func TestClaimActionMoreThanAPeriodLateRunsOneTickAndCatchesUp(t *testing.T) {
	period := 3 * time.Second
	t0 := time.Unix(1000, 0)
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 3}}
	e.startSchedule(t0)

	late := t0.Add(2*period + 500*time.Millisecond)
	if run, remove := e.claimAction(late); !run || remove {
		t.Fatalf("late claim = run %v remove %v, want run=true remove=false", run, remove)
	}
	if got, want := e.dueAt(), t0.Add(2*period); !got.Equal(want) {
		t.Fatalf("after the late claim dueAt = %v, want %v", got.Sub(t0), want.Sub(t0))
	}
	if run, _ := e.claimAction(late.Add(sweep)); !run {
		t.Fatal("next sweep did not catch up the overdue second tick")
	}
	if got, want := e.dueAt(), t0.Add(3*period); !got.Equal(want) {
		t.Fatalf("after catching up dueAt = %v, want %v", got.Sub(t0), want.Sub(t0))
	}
	if got := e.Remaining(); got != 3 {
		t.Fatalf("Remaining() = %d, want 3 after two ticks", got)
	}
}
