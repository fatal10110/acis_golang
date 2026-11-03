package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// TestPetCalledOutShortArmsRegenAtSpawn pins the start of a summon's
// regeneration phase for vitals restored below max: the phase arms as the
// pet enters the world, before any sweep runs, and its first tick lands
// within one period of the spawn.
func TestPetCalledOutShortArmsRegenAtSpawn(t *testing.T) {
	t.Parallel()
	h := bootSavedWolf(t, pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal})
	if !h.srv.DrivesClock() {
		t.Skip("pinning the regeneration phase needs the driven clock")
	}
	wolf, _ := h.spawnWolf(t)
	hp := wolf.HP()
	if hp >= wolf.MaxHPValue() {
		t.Fatalf("restored HP = %v, want below max %v", hp, wolf.MaxHPValue())
	}
	if !wolf.Regen().Active() {
		t.Fatal("regeneration idle after the pet entered the world short of HP")
	}
	w := &regenPhaseWolf{h: h, wolf: wolf, regen: task.NewNPCRegen(h.srv.State)}
	// spawnWolf's reads moved the clock past the spawn, by less than a
	// period: the tick is not due yet, and is by one period from here.
	if got := w.hpAfter(t, 0); got != hp {
		t.Fatalf("HP just after the spawn = %v, want %v: regenerated early", got, hp)
	}
	if got := w.hpAfter(t, task.NPCRegenTick); got <= hp {
		t.Fatalf("HP one period after the spawn was seen = %v, want above %v", got, hp)
	}
}
