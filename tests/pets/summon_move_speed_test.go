package pets

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// petCoveredInOneSecond lets one second pass in position-update steps and
// returns how far the server moved the pet.
func petCoveredInOneSecond(t *testing.T, h *petWorld, petActor *summon.Actor) float64 {
	t.Helper()
	from := petActor.Move().Position()
	for range 10 {
		h.srv.Advance(t, 100*time.Millisecond)
	}
	return from.Distance2D(petActor.Move().Position())
}

// onPetQueue runs fn on the pet's queue and waits for it.
func onPetQueue(t *testing.T, petActor *summon.Actor, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !petActor.Queue().Post(func() { fn(); close(done) }) {
		t.Fatal("pet queue closed")
	}
	<-done
}

// TestRunSpeedDebuffSlowsPetMovement pins a summon's server-side pace to
// its live move speed: RUN_SPEED over its template run speed
// (PlayableStatus/PetStatus.getMoveSpeed), which every position update
// reads. The wolf's DEX 30 run-speed bonus of 1.1 puts it at 132 per
// second on a fear walk; a halving RUN_SPEED debuff landing mid-walk slows
// the same walk to 66, and removing it restores 132.
func TestRunSpeedDebuffSlowsPetMovement(t *testing.T) {
	t.Parallel()
	const run = 132.0
	h, petActor, hostile := bootWolfStriker(t)
	landPetFear(t, petActor, hostile, 1092, 6, 2)
	if !petActor.IsMoving() {
		t.Fatal("IsMoving() = false after fear landed, want the flee walk")
	}
	drainFrames(t, h.client)

	requireCovered := func(what string, want float64) {
		t.Helper()
		if got := petCoveredInOneSecond(t, h, petActor); math.Abs(got-want) > 2 {
			t.Fatalf("%s: pet moved %.1f in one second, want %.1f", what, got, want)
		}
	}
	requireCovered("fleeing", run)

	slow, err := effect.New(effect.Skill{ID: 102, Level: 1, Debuff: true}, modelskill.EffectTemplate{
		Name: "Debuff", Time: 30, StackType: "speed_down", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "runSpd", Value: 0.5}},
	})
	if err != nil {
		t.Fatalf("effect.New(slow): %v", err)
	}
	slow.Effector, slow.Effected = hostile, petActor
	onPetQueue(t, petActor, func() { petActor.EffectList().Add(slow) })
	requireCovered("fleeing under the slow", run/2)

	onPetQueue(t, petActor, func() { petActor.EffectList().Remove(slow) })
	requireCovered("fleeing after the slow ends", run)
}
