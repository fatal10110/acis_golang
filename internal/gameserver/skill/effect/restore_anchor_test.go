package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// A restored effect's schedule runs from its restore instant: started 5s
// later, the 10s-period effect saved 4s into a period with 3 ticks left
// still has its first tick ahead, due 1s after the start.
func TestSeedRestoreRunsFromTheRestoreInstant(t *testing.T) {
	at := time.Unix(1000, 0)
	start := at.Add(5 * time.Second)
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 10}}
	e.seedRestore(3, 4, at)
	e.startSchedule(start)

	if got := e.Remaining(); got != 3 {
		t.Fatalf("Remaining() = %d, want 3: no tick has come due yet", got)
	}
	if run, _ := e.claimAction(start.Add(999 * time.Millisecond)); run {
		t.Fatal("claimAction fired before the first tick, 6s after the restore")
	}
	if run, remove := e.claimAction(start.Add(time.Second)); !run || remove {
		t.Fatalf("claimAction 6s after the restore = run %v remove %v, want run=true remove=false", run, remove)
	}
}

// A no-action effect whose first tick came due between its restore and its
// start has ended, whatever count it had left: List.tickAt removes an
// in-use effect whose tick action reports false.
func TestSeedRestoreEndsAnEffectWhoseFirstTickPassed(t *testing.T) {
	at := time.Unix(1000, 0)
	start := at.Add(7 * time.Second)
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 10}}
	e.seedRestore(5, 4, at)
	e.startSchedule(start)

	if got := e.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %d, want 0: the first tick, 6s after the restore, ended it", got)
	}
	if run, remove := e.claimAction(start); run || !remove {
		t.Fatalf("claimAction at the start = run %v remove %v, want run=false remove=true", run, remove)
	}
}

func TestResumeRestoredEndsOnTheFirstTick(t *testing.T) {
	tmpl := modelskill.EffectTemplate{Count: 5, Time: 10}
	at := time.Unix(1000, 0)
	// Five ticks left, 4s in: the first comes due 6s after the restore.
	if _, _, ok := resumeRestored(tmpl, 5, 4, at, at.Add(6*time.Second)); !ok {
		t.Fatal("ended exactly as its first tick comes due, want it held until the tick runs")
	}
	if _, _, ok := resumeRestored(tmpl, 5, 4, at, at.Add(6*time.Second+time.Millisecond)); ok {
		t.Fatal("still running after its first tick came due")
	}
}

func TestRestoredSaveStateCountsTheTimeSinceTheRestore(t *testing.T) {
	templates := []modelskill.EffectTemplate{{Name: "not-a-real-effect", Count: 1, Time: 99}, {Name: "Buff", Count: 3, Time: 10}}
	at := time.Unix(1000, 0)

	count, elapsed, ok := RestoredSaveState(templates, 3, 4, at, at.Add(5*time.Second+500*time.Millisecond))
	if !ok || count != 3 || elapsed != 9 {
		t.Fatalf("RestoredSaveState 5.5s after the restore = (%d, %d, %v), want (3, 9, true)", count, elapsed, ok)
	}
	if _, _, ok := RestoredSaveState(templates, 3, 4, at, at.Add(7*time.Second)); ok {
		t.Fatal("RestoredSaveState kept a 3-tick buff whose first tick came due 6s after the restore")
	}
	if count, elapsed, ok := RestoredSaveState(templates, 3, 4, time.Time{}, at.Add(time.Hour)); !ok || count != 3 || elapsed != 4 {
		t.Fatalf("RestoredSaveState without a restore instant = (%d, %d, %v), want the saved (3, 4, true)", count, elapsed, ok)
	}
}
