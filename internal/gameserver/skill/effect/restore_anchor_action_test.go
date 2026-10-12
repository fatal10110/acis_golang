package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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
