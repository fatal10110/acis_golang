package vitals

import (
	"testing"
	"time"
)

// TestGoldenAnswers pins the lookups to the golden's own lines, so a golden
// that loses a line or changes format fails here and not in a behavior test.
func TestGoldenAnswers(t *testing.T) {
	if got := DoorStage(t, 600, 350); got != 2 {
		t.Errorf("DoorStage(600, 350) = %d, want 2", got)
	}
	if got := DoorStage(t, 600, 0); got != 6 {
		t.Errorf("DoorStage(600, 0) = %d, want 6", got)
	}
	if got := DoorRegenPeriod(t); got != 5*time.Minute {
		t.Errorf("DoorRegenPeriod = %v, want 5m", got)
	}
	ticks := DoorRegenTicks(t, 1, 595)
	if last := ticks[len(ticks)-1]; last.HP != 600 || last.Active {
		t.Errorf("last regeneration tick = %+v, want 600 with the task stopped", last)
	}
	if got := RevivedHP(t, 632); got != 442.4 {
		t.Errorf("RevivedHP(632) = %v, want 442.4", got)
	}
	if got := PetMaxHP(t, 400, 43); got != 632 {
		t.Errorf("PetMaxHP(400, 43) = %d, want 632", got)
	}
	if got := PetExpRestored(t, 510, 481, 100); got != 29 {
		t.Errorf("PetExpRestored(510, 481, 100) = %d, want 29", got)
	}
	if hp, mp := PhoenixRevivedVitals(t, 410, 200); hp != 410 || mp != 200 {
		t.Errorf("PhoenixRevivedVitals(410, 200) = %v, %v, want both at the maximum", hp, mp)
	}
	if got := RespawnedDoorHP(t, 601, true, true); got != 300 {
		t.Errorf("RespawnedDoorHP(601, weak, dead) = %v, want 300", got)
	}
	if got := PetDeath(t, 10, 200, 500, 1000, 510); got != (PetDeathResult{Lost: 29, Before: 510, After: 481, Level: 9}) {
		t.Errorf("PetDeath(10, exp 510) = %+v", got)
	}
}
