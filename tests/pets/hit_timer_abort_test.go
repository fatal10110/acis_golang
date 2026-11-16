package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPetSwingAbortedAtItsHitGoesIdle pins the hit timer of a pet swing
// whose target a third party killed in flight (CreatureAttack.onHitTimer
// stop(), PlayableAttack.stop's tryToIdle): nothing lands and the pet goes
// idle, which for a pet following its owner means following it again. Its
// owner is told nothing: a summon's clientActionFailed is a no-op.
func TestPetSwingAbortedAtItsHitGoesIdle(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, nil)
	if !h.srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	petActor, _ := h.spawnWolf(t)
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)
	if !hostile.Die(nil, nil) {
		t.Fatal("hostile.Die() = false, want the third-party kill")
	}

	h.srv.AdvanceUntil(t, "the pet's swing ends", func() bool { return !petActor.IsAttackingNow() })
	h.handled(t)
	frames := drainFrames(t, h.client)
	if n := countOpcode(frames, serverpackets.OpcodeActionFailed); n != 0 {
		t.Fatalf("ActionFailed frames for the pet's aborted swing = %d, want 0", n)
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent after the aborted swing = %v, want follow-owner (idle)", got)
	}
}
