package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// A restored effect whose ticks run an action keeps running from the
// replay: ticks due on the loading screen would pass without their damage
// or drain, so a held or dropped loading screen must not shorten it.
func TestRestoreAnchorKeepsTickActionEffectsOnTheReplay(t *testing.T) {
	at := time.Unix(1000, 0)
	for _, tc := range []struct {
		name string
		want time.Time
	}{
		{"DamOverTime", time.Time{}},
		{"ManaDamOverTime", time.Time{}},
		{"Fear", time.Time{}},
		{"Buff", at},
		{"Debuff", at},
		{"Stun", at},
	} {
		if got := restoreAnchor(modelskill.EffectTemplate{Name: tc.name, Count: 10, Time: 3}, at); !got.Equal(tc.want) {
			t.Errorf("restoreAnchor(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A saved 3s-period poison with all ten ticks left, dropped 40s after the
// restore — past its whole 30s run — saves as it was read back.
func TestRestoredSaveStateKeepsATickActionEffectWhole(t *testing.T) {
	templates := []modelskill.EffectTemplate{{Name: "DamOverTime", Count: 10, Time: 3, Value: 5}}
	at := time.Unix(1000, 0)
	count, elapsed, ok := RestoredSaveState(templates, 10, 1, at, at.Add(40*time.Second))
	if !ok || count != 10 || elapsed != 1 {
		t.Fatalf("RestoredSaveState for a poison 40s after the restore = (%d, %d, %v), want the saved (10, 1, true)", count, elapsed, ok)
	}
}
