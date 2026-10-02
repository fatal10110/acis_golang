package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// A restored effect's schedule runs from its restore instant: started 7s
// later, the 10s-period effect saved 4s into a period with 3 ticks left
// has had one tick (due at +6s) pass, leaving 2 ticks and 9s to the next.
func TestSeedRestoreRunsFromTheRestoreInstant(t *testing.T) {
	at := time.Unix(1000, 0)
	start := at.Add(7 * time.Second)
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 10}}
	e.seedRestore(3, 4, at)
	e.startSchedule(start)

	if got := e.Remaining(); got != 2 {
		t.Fatalf("Remaining() = %d, want 2: the tick due 6s after the restore has passed", got)
	}
	if run, _ := e.claimAction(start.Add(8 * time.Second)); run {
		t.Fatal("claimAction fired before the next tick, 16s after the restore")
	}
	if run, remove := e.claimAction(start.Add(9 * time.Second)); !run || remove {
		t.Fatalf("claimAction 16s after the restore = run %v remove %v, want run=true remove=false", run, remove)
	}
}

func TestResumeRestoredEndsOnTheLastTick(t *testing.T) {
	tmpl := modelskill.EffectTemplate{Count: 5, Time: 10}
	at := time.Unix(1000, 0)
	// Two ticks left, 4s in: they come due 6s and 16s after the restore.
	if _, _, ok := resumeRestored(tmpl, 2, 4, at, at.Add(16*time.Second)); !ok {
		t.Fatal("ended exactly as its last tick comes due, want it held until the tick runs")
	}
	if _, _, ok := resumeRestored(tmpl, 2, 4, at, at.Add(16*time.Second+time.Millisecond)); ok {
		t.Fatal("still running after its last tick came due")
	}
}

func TestRestoredSaveStateCountsTheTimeSinceTheRestore(t *testing.T) {
	templates := []modelskill.EffectTemplate{{Name: "not-a-real-effect", Count: 1, Time: 99}, {Name: "Buff", Count: 3, Time: 10}}
	at := time.Unix(1000, 0)

	count, elapsed, ok := RestoredSaveState(templates, 3, 4, at, at.Add(7*time.Second+500*time.Millisecond))
	if !ok || count != 2 || elapsed != 1 {
		t.Fatalf("RestoredSaveState 7.5s after the restore = (%d, %d, %v), want (2, 1, true)", count, elapsed, ok)
	}
	if _, _, ok := RestoredSaveState(templates, 1, 4, at, at.Add(7*time.Second)); ok {
		t.Fatal("RestoredSaveState kept an effect whose last tick came due 6s after the restore")
	}
	if count, elapsed, ok := RestoredSaveState(templates, 3, 4, time.Time{}, at.Add(time.Hour)); !ok || count != 3 || elapsed != 4 {
		t.Fatalf("RestoredSaveState without a restore instant = (%d, %d, %v), want the saved (3, 4, true)", count, elapsed, ok)
	}
}
