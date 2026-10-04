package pets

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// TestPetRegenFirstTickThreeSecondsAfterDrop pins a summon's regeneration
// phase to its own HP drop, as a monster's and a player's: a full wolf
// knocked down to 50 HP regenerates nothing for 3s, then takes its 3.1284 HP
// tick (see TestPetRegeneratesOnEachRegenTick for the oracle) at exactly 3s.
func TestPetRegenFirstTickThreeSecondsAfterDrop(t *testing.T) {
	t.Parallel()
	h := bootSavedWolf(t, pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal})
	if !h.srv.DrivesClock() {
		t.Skip("pinning the regeneration phase needs the driven clock")
	}
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	onWolf := func(fn func()) {
		t.Helper()
		if !wolf.Queue().Post(fn) {
			t.Fatal("post: wolf queue closed")
		}
		h.srv.Settle(t)
	}
	onWolf(func() { // full: the restore's phase stops
		wolf.AddMP(wolf.MaxMPValue())
		wolf.SetHP(wolf.MaxHPValue())
	})
	if wolf.Regen().Active() {
		t.Fatal("regeneration still armed at full HP and MP")
	}
	onWolf(func() { wolf.SetHP(50) })

	regen := task.NewNPCRegen(h.srv.State)
	hpAfter := func(d time.Duration) float64 {
		t.Helper()
		h.srv.Advance(t, d)
		regen.Tick()
		h.srv.Settle(t)
		return wolf.HP()
	}
	if got := hpAfter(task.NPCRegenTick - time.Millisecond); got != 50 {
		t.Fatalf("HP 2.999s after the drop = %v, want 50: regenerated early", got)
	}
	if got, want := hpAfter(time.Millisecond), 50+3.1284; math.Abs(got-want) > 1e-9 {
		t.Fatalf("HP 3s after the drop = %v, want %v", got, want)
	}
}
