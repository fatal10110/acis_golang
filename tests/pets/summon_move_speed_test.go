package pets

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// petCoveredInOneSecond lets one second pass in position-update ticks and
// returns how far the server moved the pet.
func petCoveredInOneSecond(t *testing.T, h *petWorld, petActor *summon.Actor) float64 {
	t.Helper()
	from := petActor.Move().Position()
	for range 10 {
		h.srv.Advance(t, 100*time.Millisecond)
		h.srv.TickPositions()
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
//
// The client animates the base run speed times the movement speed
// multiplier (PetInfo.java, AbstractNpcInfo.SummonInfo:
// getMovementSpeedMultiplier). Each speed change republishes the owner's
// PetInfo and the observer's NpcInfo, and both must show the pace the
// server measures.
func TestRunSpeedDebuffSlowsPetMovement(t *testing.T) {
	t.Parallel()
	const run = 132.0
	h, petActor, hostile := bootWolfStriker(t)
	observer := h.joinSecondPlayer(t, "Watcher")
	landPetFear(t, petActor, hostile, 1092, 6, 2)
	if !petActor.IsMoving() {
		t.Fatal("IsMoving() = false after fear landed, want the flee walk")
	}
	drainFrames(t, h.client)
	drainFrames(t, observer.client)

	// The frames are read after each pace measurement: the flee's arrival
	// runs on the wall clock, which a drain would spend.
	requireClientPace := func(what string, want float64) {
		t.Helper()
		ownerPace, ok := lastPetInfoPace(t, drainFrames(t, h.client), petActor.ObjectID())
		if !ok || math.Abs(ownerPace-want) > 0.01 {
			t.Fatalf("%s: owner's PetInfo pace %.2f (seen %v), want %.1f", what, ownerPace, ok, want)
		}
		observerPace, ok := lastSummonNpcInfoPace(t, drainFrames(t, observer.client), petActor.ObjectID())
		if !ok || math.Abs(observerPace-want) > 0.01 {
			t.Fatalf("%s: observer's NpcInfo pace %.2f (seen %v), want %.1f", what, observerPace, ok, want)
		}
	}

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
	requireClientPace("slow landing", run/2)

	onPetQueue(t, petActor, func() { petActor.EffectList().Remove(slow) })
	requireCovered("fleeing after the slow ends", run)
	requireClientPace("slow ending", run)
}

// lastPetInfoPace returns the base run speed times the movement speed
// multiplier of the last PetInfo for objectID among frames.
func lastPetInfoPace(t *testing.T, frames [][]byte, objectID int32) (pace float64, seen bool) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodePetInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadInt32() // summon type
		if r.ReadInt32() != objectID {
			continue
		}
		pace, seen = readSpeedsAndMultiplier(t, r, 9), true
	}
	return pace, seen
}

// lastSummonNpcInfoPace returns the base run speed times the movement
// speed multiplier of the last NpcInfo for objectID among frames.
func lastSummonNpcInfoPace(t *testing.T, frames [][]byte, objectID int32) (pace float64, seen bool) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		pace, seen = readSpeedsAndMultiplier(t, r, 9), true
	}
	return pace, seen
}

// readSpeedsAndMultiplier skips skip int32 fields, then reads the eight
// run/walk speed pairs and the movement speed multiplier, returning the run
// speed scaled by it after checking every pair carries the same speeds.
func readSpeedsAndMultiplier(t *testing.T, r *wire.Reader, skip int) float64 {
	t.Helper()
	for range skip {
		r.ReadInt32()
	}
	run, walk := r.ReadInt32(), r.ReadInt32()
	for range 3 {
		if gotRun, gotWalk := r.ReadInt32(), r.ReadInt32(); gotRun != run || gotWalk != walk {
			t.Fatalf("speed pair = %d/%d, want %d/%d in every pair", gotRun, gotWalk, run, walk)
		}
	}
	multiplier := r.ReadFloat64()
	if err := r.Err(); err != nil {
		t.Fatalf("read speeds: %v", err)
	}
	return float64(run) * multiplier
}
