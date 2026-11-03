package ai

import "testing"

// An attack requested mid-swing is the next intention
// (PlayableAI.tryToAttack, PlayableAI.java:243-250) until the swing's end
// runs it.
func TestPlayerAttackHasNextMidSwingRequest(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	first, second := gatePlayerFake(2, 40, 500), gatePlayerFake(3, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	swingingAt(t, brain, strike, first)
	if brain.HasNext() {
		t.Fatal("HasNext() during the opening swing = true, want nothing queued")
	}

	brain.Start(second, false)
	if !brain.HasNext() {
		t.Fatal("HasNext() after a mid-swing attack request = false, want the queued attack")
	}

	strike.attackingNow = false
	brain.FinishedAttack()
	if brain.HasNext() {
		t.Fatal("HasNext() after the queued attack ran = true, want it current")
	}
}

// A think that finds the swing in flight re-queues the current attack as the
// next intention (PlayerAI.thinkAttack, PlayerAI.java:201-206).
func TestPlayerAttackHasNextThinkFindsSwing(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	first := gatePlayerFake(2, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	swingingAt(t, brain, strike, first)

	brain.Think()
	if !brain.HasNext() {
		t.Fatal("HasNext() after a think mid-swing = false, want the attack re-queued")
	}
	brain.DropQueued()
	if brain.HasNext() {
		t.Fatal("HasNext() after DropQueued() = true, want nothing queued")
	}
}

// An attack requested mid-cast waits as the next intention until the cast
// ends and runs it.
func TestPlayerAttackHasNextMidCastRequest(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	pc.casting = true

	brain.Start(target, false)
	if !brain.HasNext() {
		t.Fatal("HasNext() after a mid-cast attack request = false, want the attack waiting on the cast")
	}

	pc.casting = false
	brain.ResumeAfterCast(false)
	if brain.HasNext() {
		t.Fatal("HasNext() after the cast ran the attack = true, want it current")
	}
}
