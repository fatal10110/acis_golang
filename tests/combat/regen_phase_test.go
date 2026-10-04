package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestNPCRegenFirstTickThreeSecondsAfterHit pins a monster's regeneration
// phase to its own HP drop (CreatureStatus.setHp starting
// scheduleAtFixedRate(doRegeneration, 3000, 3000) on the first drop below
// max, stopHpMpRegeneration at full):
//   - nothing regenerates before 3s after the hit, then one tick lands at 3s;
//   - a second hit while the phase runs neither delays nor resets it;
//   - back at full the phase stops, and the next hit starts a fresh one.
//
// The fixture monster regenerates the 1 HP floor a tick.
func TestNPCRegenFirstTickThreeSecondsAfterHit(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("pinning the regeneration phase needs the driven clock")
	}
	hostile := srv.SpawnHostileNPC(t)
	regen := task.NewNPCRegen(srv.State)
	hpAfter := func(d time.Duration) int {
		t.Helper()
		srv.Advance(t, d)
		regen.Tick()
		srv.Settle(t)
		return hostile.CurrentHP()
	}
	hit := func(dmg float64) {
		t.Helper()
		onQueue(t, hostile.Queue(), func() { hostile.ConsumeHP(dmg) })
	}

	full := hostile.CurrentHP()
	if got := hpAfter(1100 * time.Millisecond); got != full { // off any global 3s grid
		t.Fatalf("unhurt monster HP = %d, want %d", got, full)
	}
	hit(100)
	if got := hpAfter(2900 * time.Millisecond); got != full-100 {
		t.Fatalf("HP 2.9s after the hit = %d, want %d: regenerated early", got, full-100)
	}
	hit(100) // while the phase runs
	if got := hpAfter(100 * time.Millisecond); got != full-199 {
		t.Fatalf("HP 3s after the first hit = %d, want %d: one tick on the first hit's phase", got, full-199)
	}
	if got := hpAfter(2999 * time.Millisecond); got != full-199 {
		t.Fatalf("HP 5.999s after the first hit = %d, want %d", got, full-199)
	}
	if got := hpAfter(time.Millisecond); got != full-198 {
		t.Fatalf("HP 6s after the first hit = %d, want %d: the next tick, one period on", got, full-198)
	}

	onQueue(t, hostile.Queue(), func() { hostile.SetHP(hostile.MaxHPValue()) })
	if hostile.Regen().Active() {
		t.Fatal("regeneration still armed at full HP")
	}
	if got := hpAfter(1300 * time.Millisecond); got != full {
		t.Fatalf("HP back at full = %d, want %d", got, full)
	}
	hit(100)
	if got := hpAfter(2999 * time.Millisecond); got != full-100 {
		t.Fatalf("HP 2.999s after the new hit = %d, want %d: ticked on the old phase", got, full-100)
	}
	if got := hpAfter(time.Millisecond); got != full-99 {
		t.Fatalf("HP 3s after the new hit = %d, want %d", got, full-99)
	}
}
