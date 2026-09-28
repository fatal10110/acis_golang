package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// petMoveDest returns the destination of a MoveToLocation frame when it
// moves objectID.
func petMoveDest(frames [][]byte, objectID int32) (location.Location, bool) {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMoveToLocation {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}, true
	}
	return location.Location{}, false
}

// landPetFear applies a real Fear effect from the monster to the pet on the
// pet's queue and waits for its start hook to finish.
func landPetFear(t *testing.T, petActor *summon.Actor, effector effect.Actor, skillID modelskill.ID, count, period int) *effect.Effect {
	t.Helper()
	fear, err := effect.New(
		effect.Skill{ID: skillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "Fear", Count: count, Time: period, Icon: true},
	)
	if err != nil {
		t.Fatalf("effect.New(Fear): %v", err)
	}
	fear.Effector, fear.Effected = effector, petActor
	done := make(chan struct{})
	if !petActor.Queue().Post(func() { petActor.EffectList().Add(fear); close(done) }) {
		t.Fatal("post fear: queue closed")
	}
	<-done
	return fear
}

// TestFearedPetFleesOnceThenStaysPut pins fear on a pet. Fear (1092) runs at
// half its count on a playable. The landing walks the pet 500 units away from
// the monster; later ticks find it already afraid and leave it in place.
func TestFearedPetFleesOnceThenStaysPut(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	origin := petActor.Move().Position()
	fear := landPetFear(t, petActor, hostile, 1092, 6, 2)

	if !petActor.Afraid() {
		t.Fatal("Afraid() = false after fear landed, want true")
	}
	if got := fear.Remaining(); got != 3 {
		t.Fatalf("Remaining() = %d for Fear on a pet, want the halved 3", got)
	}
	hx, hy, _ := hostile.Position()
	dest, ok := petMoveDest(drainFrames(t, h.client), petActor.ObjectID())
	if !ok {
		t.Fatal("fear landing sent no flee MoveToLocation for the pet")
	}
	if want := origin.FleeFrom(hx, hy, 500); dest.X != want.X || dest.Y != want.Y {
		t.Fatalf("pet flee dest = %+v from %+v, want %+v", dest, origin, want)
	}

	h.srv.Advance(t, 2*time.Second)
	h.srv.TickEffects()
	if _, moved := petMoveDest(drainFrames(t, h.client), petActor.ObjectID()); moved {
		t.Fatal("tick flee moved an already-afraid pet, want it left in place")
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent while afraid = %v, want follow-owner", got)
	}
}

// TestPetFollowsOwnerOnceFleeEndsAfterFear pins the end of a flee walk that
// outlasts its fear: the walk's arrival sends the pet idle, and an idle pet
// that follows its owner walks back to it.
func TestPetFollowsOwnerOnceFleeEndsAfterFear(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	landPetFear(t, petActor, hostile, 101, 1, 1)
	if _, ok := petMoveDest(drainFrames(t, h.client), petActor.ObjectID()); !ok {
		t.Fatal("fear landing sent no flee MoveToLocation for the pet")
	}

	h.srv.Advance(t, time.Second)
	h.srv.TickEffects()
	if petActor.Afraid() {
		t.Fatal("Afraid() = true after the single tick, want the fear gone")
	}
	if !petActor.IsMoving() {
		t.Fatal("IsMoving() = false once the fear ended, want the flee walk still running")
	}
	drainFrames(t, h.client)

	h.srv.Advance(t, 5*time.Second)
	if !petMovesTo(drainFrames(t, h.client), petActor.ObjectID()) {
		t.Fatal("pet stayed where its flee ended, want it walking back to its owner")
	}
}

// petMovesTo reports whether frames start a walk for objectID, toward a pawn
// or a location.
func petMovesTo(frames [][]byte, objectID int32) bool {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMoveToPawn && frame[0] != serverpackets.OpcodeMoveToLocation {
			continue
		}
		if wire.NewReader(frame[1:]).ReadInt32() == objectID {
			return true
		}
	}
	return false
}
