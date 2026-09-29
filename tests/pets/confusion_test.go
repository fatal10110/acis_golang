package pets

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// confusionSkillID is the Confusion skill whose effect the pet receives.
const confusionSkillID = 2

// attacksOn reports whether frames carry an Attack by attackerID whose first
// hit targets targetID.
func attacksOn(frames [][]byte, attackerID, targetID int32) bool {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeAttack {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() == attackerID && r.ReadInt32() == targetID {
			return true
		}
	}
	return false
}

// TestConfusedPetAttacksItsOwner lands Confusion on a pet whose only
// neighbour within 1000 is its owner. A playable is a confusion candidate,
// so the pet turns its attack on the owner through its own AI.
func TestConfusedPetAttacksItsOwner(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	petActor, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	landFearPetEffect(t, petActor, petActor, "Confusion", confusionSkillID, 1, 1)
	if frames := drainFrames(t, h.client); !attacksOn(frames, petActor.ObjectID(), h.ownerID) {
		t.Fatalf("confused pet sent no Attack on its owner %d: opcodes %x", h.ownerID, frameOpcodes(frames))
	}
}

// TestConfusedPetAttacksNearbyMonster moves a pet 1500 away from its owner
// next to a monster, so the monster is its only confusion candidate, and
// the pet attacks it.
func TestConfusedPetAttacksNearbyMonster(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	petActor, _ := h.spawnWolf(t)
	ox, oy, oz := h.srv.PlayerPosition(t, h.ownerID)
	far := location.Location{X: ox + 1500, Y: oy, Z: oz}
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: far.X + 40, Y: far.Y, Z: far.Z})
	movePet(t, petActor, far)
	drainUntilQuiet(t, h.client)

	landFearPetEffect(t, petActor, hostile, "Confusion", confusionSkillID, 1, 1)
	if frames := drainFrames(t, h.client); !attacksOn(frames, petActor.ObjectID(), hostile.ObjectID()) {
		t.Fatalf("confused pet sent no Attack on the monster: opcodes %x", frameOpcodes(frames))
	}
}

// TestPetFollowsOwnerWhenConfusionEnds moves a pet 1500 away from its owner
// with nobody near it, so its confusion finds no candidate and it stays put.
// When the effect ends the pet walks back to follow its owner.
func TestPetFollowsOwnerWhenConfusionEnds(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	petActor, _ := h.spawnWolf(t)
	ox, oy, oz := h.srv.PlayerPosition(t, h.ownerID)
	movePet(t, petActor, location.Location{X: ox + 1500, Y: oy, Z: oz})
	drainUntilQuiet(t, h.client)

	confusion := landFearPetEffect(t, petActor, petActor, "Confusion", confusionSkillID, 1, 1)
	if frames := drainFrames(t, h.client); petMovesTo(frames, petActor.ObjectID()) || attacksOn(frames, petActor.ObjectID(), h.ownerID) {
		t.Fatalf("confused pet with no candidate moved or attacked: opcodes %x", frameOpcodes(frames))
	}

	h.srv.Advance(t, time.Second)
	h.srv.TickEffects()
	if confusion.InUse() {
		t.Fatal("confusion still in use after its single tick")
	}
	frames := drainFrames(t, h.client)
	dest, ok := petMoveDest(frames, petActor.ObjectID())
	if !ok {
		t.Fatalf("pet did not start walking once confusion ended: opcodes %x", frameOpcodes(frames))
	}
	if d := math.Hypot(float64(dest.X-ox), float64(dest.Y-oy)); d >= petCloseToOwner {
		t.Fatalf("pet walk dest = %+v, %.0f from its owner at (%d,%d), want it heading back beside the owner", dest, d, ox, oy)
	}
}

// movePet places the pet at dest on its own queue.
func movePet(t *testing.T, petActor *summon.Actor, dest location.Location) {
	t.Helper()
	done := make(chan struct{})
	if !petActor.Queue().Post(func() { petActor.SetXYZ(dest.X, dest.Y, dest.Z); close(done) }) {
		t.Fatal("post move: queue closed")
	}
	<-done
}
