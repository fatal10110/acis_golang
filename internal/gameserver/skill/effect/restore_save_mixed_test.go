package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// A restored skill whose first effect has ended on the loading screen saves
// the state of the next effect still running, as the reference writes the
// first effect of a skill left in the list. Poison of Death (4082): once its
// Root has run out, the DamOverTime behind it keeps the row.
func TestRestoredSaveStatePassesAnEndedEffectToTheNext(t *testing.T) {
	templates := []modelskill.EffectTemplate{
		{Name: "Root", Count: 1, Time: 4200},
		{Name: "DamOverTime", Count: 4200, Time: 3},
		{Name: "ImobileBuff", Count: 1, Time: 4200},
	}
	at := time.Unix(1000, 0)

	if count, elapsed, ok := RestoredSaveState(templates, 2000, 1, at, at.Add(10*time.Second)); !ok || count != 1 || elapsed != 11 {
		t.Fatalf("RestoredSaveState 10s after the restore = (%d, %d, %v), want the running Root's (1, 11, true)", count, elapsed, ok)
	}
	if count, elapsed, ok := RestoredSaveState(templates, 2000, 1, at, at.Add(4200*time.Second)); !ok || count != 2000 || elapsed != 1 {
		t.Fatalf("RestoredSaveState after the Root ended = (%d, %d, %v), want the DamOverTime's saved (2000, 1, true)", count, elapsed, ok)
	}
}

// A skill whose every known effect has ended saves nothing; one ended effect
// passes the row on to a later effect that is still running.
func TestRestoredSaveStateEndsOnlyWhenEveryEffectHasEnded(t *testing.T) {
	templates := []modelskill.EffectTemplate{
		{Name: "Buff", Count: 1, Time: 10},
		{Name: "not-a-real-effect", Count: 1, Time: 99},
		{Name: "Debuff", Count: 1, Time: 30},
	}
	at := time.Unix(1000, 0)

	if count, elapsed, ok := RestoredSaveState(templates, 1, 4, at, at.Add(20*time.Second)); !ok || count != 1 || elapsed != 24 {
		t.Fatalf("RestoredSaveState 20s after the restore = (%d, %d, %v), want the Debuff's (1, 24, true)", count, elapsed, ok)
	}
	if _, _, ok := RestoredSaveState(templates, 1, 4, at, at.Add(27*time.Second)); ok {
		t.Fatal("RestoredSaveState kept a skill whose every effect ended")
	}
}
