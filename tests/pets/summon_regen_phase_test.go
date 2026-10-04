package pets

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// regenPhaseWolf is a called-out wolf at full HP and MP, its regeneration
// phase idle, on a driven clock, with the production regeneration sweep.
type regenPhaseWolf struct {
	h     *petWorld
	wolf  *summon.Actor
	regen *task.NPCRegen
}

func bootRegenPhaseWolf(t *testing.T) *regenPhaseWolf {
	t.Helper()
	h := bootSavedWolf(t, pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal})
	if !h.srv.DrivesClock() {
		t.Skip("pinning the regeneration phase needs the driven clock")
	}
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	w := &regenPhaseWolf{h: h, wolf: wolf, regen: task.NewNPCRegen(h.srv.State)}
	w.on(t, func() { // full: the restore's phase stops
		wolf.AddMP(wolf.MaxMPValue())
		wolf.SetHP(wolf.MaxHPValue())
	})
	if wolf.Regen().Active() {
		t.Fatal("regeneration still armed at full HP and MP")
	}
	return w
}

// on runs fn on the wolf's queue and waits for it.
func (w *regenPhaseWolf) on(t *testing.T, fn func()) {
	t.Helper()
	if !w.wolf.Queue().Post(fn) {
		t.Fatal("post: wolf queue closed")
	}
	w.h.srv.Settle(t)
}

// hpAfter advances the clock by d, runs one sweep and returns the wolf's HP.
func (w *regenPhaseWolf) hpAfter(t *testing.T, d time.Duration) float64 {
	t.Helper()
	w.h.srv.Advance(t, d)
	w.regen.Tick()
	w.h.srv.Settle(t)
	return w.wolf.HP()
}

// TestPetRegenFirstTickThreeSecondsAfterDrop pins a summon's regeneration
// phase to its own HP drop, as a monster's and a player's: a full wolf
// knocked down to 50 HP regenerates nothing for 3s, then takes its 3.1284 HP
// tick (see TestPetRegeneratesOnEachRegenTick for the oracle) at exactly 3s.
func TestPetRegenFirstTickThreeSecondsAfterDrop(t *testing.T) {
	t.Parallel()
	w := bootRegenPhaseWolf(t)
	w.on(t, func() { w.wolf.SetHP(50) })

	if got := w.hpAfter(t, task.NPCRegenTick-time.Millisecond); got != 50 {
		t.Fatalf("HP 2.999s after the drop = %v, want 50: regenerated early", got)
	}
	if got, want := w.hpAfter(t, time.Millisecond), 50+3.1284; math.Abs(got-want) > 1e-9 {
		t.Fatalf("HP 3s after the drop = %v, want %v", got, want)
	}
}

// TestPetRegenFirstTickThreeSecondsAfterRevive pins the revive's start of
// the phase (Creature.doRevive's setHp starts the regeneration task): death
// stopped it, and the revive below max HP arms it at once, its first tick
// exactly one period later, not whenever a backstop pass notices the wolf.
func TestPetRegenFirstTickThreeSecondsAfterRevive(t *testing.T) {
	t.Parallel()
	w := bootRegenPhaseWolf(t)
	owner, ok := w.h.srv.State.Player(w.h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}
	w.on(t, func() {
		w.wolf.ReduceHP(w.wolf.MaxHPValue()*2, owner.(attackable.Combatant), modelskill.Definition{})
	})
	if !w.wolf.Dead() || w.wolf.Regen().Active() {
		t.Fatalf("after the lethal hit: dead %v, regeneration armed %v; want dead and idle", w.wolf.Dead(), w.wolf.Regen().Active())
	}
	w.h.srv.Advance(t, 1500*time.Millisecond) // off any earlier grid
	w.on(t, func() {
		if !w.wolf.Revive() {
			t.Error("Revive() = false for a dead wolf")
		}
	})
	revived := w.wolf.HP()
	if revived <= 0 || revived >= w.wolf.MaxHPValue() {
		t.Fatalf("revived HP = %v, want below max %v", revived, w.wolf.MaxHPValue())
	}
	if !w.wolf.Regen().Active() {
		t.Fatal("revive below max HP left regeneration idle")
	}
	if got := w.hpAfter(t, task.NPCRegenTick-time.Millisecond); got != revived {
		t.Fatalf("HP 2.999s after the revive = %v, want %v: regenerated early", got, revived)
	}
	if got := w.hpAfter(t, time.Millisecond); got <= revived {
		t.Fatalf("HP 3s after the revive = %v, want above %v", got, revived)
	}
}

// TestPetLevelUpRefillRestartsRegenPhase pins the level-up refill's stop of
// the phase (PlayableStatus.addLevel's setMaxHpMp): a wolf hit, refilled by
// a level-up before its first tick and hit again takes no tick on the first
// hit's grid, only one period after the second hit.
func TestPetLevelUpRefillRestartsRegenPhase(t *testing.T) {
	t.Parallel()
	w := bootRegenPhaseWolf(t)
	w.on(t, func() { w.wolf.SetHP(50) })
	if got := w.hpAfter(t, 2500*time.Millisecond); got != 50 {
		t.Fatalf("HP 2.5s after the first hit = %v, want 50", got)
	}
	w.on(t, func() { w.wolf.AddExpAndSp(wolfNextLevelExp, 0) })
	if w.wolf.Level() <= wolfLevel || w.wolf.HP() != w.wolf.MaxHPValue() {
		t.Fatalf("after the level-up: level %d HP %v, want above %d at max HP %v", w.wolf.Level(), w.wolf.HP(), wolfLevel, w.wolf.MaxHPValue())
	}
	if w.wolf.Regen().Active() {
		t.Fatal("regeneration still armed after the level-up refill")
	}
	w.h.srv.Advance(t, 400*time.Millisecond)
	w.on(t, func() { w.wolf.SetHP(50) }) // second hit at 2.9s
	if got := w.hpAfter(t, 100*time.Millisecond); got != 50 {
		t.Fatalf("HP at the first hit's 3s mark = %v, want 50: ticked on the stale phase", got)
	}
	if got := w.hpAfter(t, task.NPCRegenTick-100*time.Millisecond-time.Millisecond); got != 50 {
		t.Fatalf("HP 2.999s after the second hit = %v, want 50", got)
	}
	if got := w.hpAfter(t, time.Millisecond); got <= 50 {
		t.Fatalf("HP 3s after the second hit = %v, want above 50", got)
	}
}
