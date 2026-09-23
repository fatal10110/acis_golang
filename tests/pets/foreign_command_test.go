package pets

import "testing"

// petStopAction is the pet shortcut that cancels the pet's current intention.
const petStopAction = int32(17)

// TestPetTargetAndIntentSurviveAnotherActorsCommands drives the pet's
// target and intention from two queues at once: the owner's attack and stop
// shortcuts run on the owner's queue, while an aggressor's skill landing on
// the pet (an aggression debuff retargeting it, a stun idling it) runs on the
// aggressor's queue. Run under -race on the real pool, an unguarded target
// or intention field fails here.
func TestPetTargetAndIntentSurviveAnotherActorsCommands(t *testing.T) {
	h := bootOwnerWithCollar(t)
	petActor, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)

	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)

	for range 50 {
		hostile.Queue().Post(func() {
			if petActor.CurrentTarget() == nil {
				petActor.SetTarget(hostile)
			}
			petActor.TryToIdle()
		})
		h.client.Send(encodeRequestActionUse(petAttackAction, false))
		h.client.Send(encodeRequestActionUse(petStopAction, false))
	}
	h.srv.Settle(t)
	drainUntilQuiet(t, h.client)

	if got := petActor.CurrentTarget(); got == nil || got.ObjectID() != hostile.ObjectID() {
		t.Fatalf("pet target = %v, want the monster %d", got, hostile.ObjectID())
	}
}
