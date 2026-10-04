package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// A restored effect runs its schedule from the restore instant, an effect
// whose ticks run an action included: the replay runs the actions of the
// ticks due in between (#3266). Only a damage-over-time tick that may kill
// still waits for the replay (#3394).
func TestRestoreAnchorRunsEveryNonLethalEffectFromTheRestore(t *testing.T) {
	at := time.Unix(1000, 0)
	for _, tc := range []struct {
		name      string
		killByDOT bool
		want      time.Time
	}{
		{"DamOverTime", false, at},
		{"DamOverTime", true, time.Time{}},
		{"ManaDamOverTime", true, at},
		{"Fear", false, at},
		{"Buff", false, at},
		{"Debuff", true, at},
		{"Stun", false, at},
	} {
		meta := Skill{KillByDOT: tc.killByDOT}
		if got := restoreAnchor(meta, modelskill.EffectTemplate{Name: tc.name, Count: 10, Time: 3}, at); !got.Equal(tc.want) {
			t.Errorf("restoreAnchor(%s, killByDOT %v) = %v, want %v", tc.name, tc.killByDOT, got, tc.want)
		}
	}
	if got := restoreAnchor(Skill{}, modelskill.EffectTemplate{Name: "Buff"}, time.Time{}); !got.IsZero() {
		t.Errorf("restoreAnchor without a restore instant = %v, want zero (the replay)", got)
	}
}

// A saved 3s-period poison with all ten ticks left, saved 40s after the
// restore — past its whole 30s run — without a replay having run its ticks,
// saves as it was read back: only a replay spends them, with their damage.
func TestRestoredSaveStateKeepsATickActionEffectWhole(t *testing.T) {
	templates := []modelskill.EffectTemplate{{Name: "DamOverTime", Count: 10, Time: 3, Value: 5}}
	at := time.Unix(1000, 0)
	count, elapsed, ok := RestoredSaveState(templates, 10, 1, at, at.Add(40*time.Second))
	if !ok || count != 10 || elapsed != 1 {
		t.Fatalf("RestoredSaveState for a poison 40s after the restore = (%d, %d, %v), want the saved (10, 1, true)", count, elapsed, ok)
	}
}
