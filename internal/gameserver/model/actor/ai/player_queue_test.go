package ai

import "testing"

// swingingAt starts an attack on the first target and leaves its swing in
// flight.
func swingingAt(t *testing.T, brain *PlayerAttack, strike *recordingAttack, first *gateFake) {
	t.Helper()
	if !brain.Start(first, false) {
		t.Fatal("Start() = false, want the first swing")
	}
	strike.attackingNow = true
}

// An attack on another target requested mid-swing is the next intention
// (PlayableAI.tryToAttack, PlayableAI.java:243-250): the attack the swing is
// for stays current until the swing ends, then the queued one runs.
func TestPlayerAttackMidSwingRequestQueuesBehindCurrent(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	first, second := gatePlayerFake(2, 40, 500), gatePlayerFake(3, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	swingingAt(t, brain, strike, first)

	if brain.Start(second, false) {
		t.Fatal("Start() mid-swing = true, want queued with ActionFailed")
	}
	if got := brain.Target(); got != first {
		t.Fatalf("Target() mid-swing = %v, want the swing's target", got)
	}

	strike.attackingNow = false
	brain.FinishedAttack()
	if strike.doAttackCalls != 2 || strike.target != second {
		t.Fatalf("swings = %d, last at %v, want the queued attack's swing", strike.doAttackCalls, strike.target)
	}
	if got := brain.Target(); got != second {
		t.Fatalf("Target() after the swing = %v, want the queued target", got)
	}
}

// Clearing only the next intention mid-swing (tryToIdle's busy branch,
// PlayableAI.java:362-367) leaves the current attack, which the swing's end
// re-thinks (PlayableAI.onEvtFinishedAttack, PlayableAI.java:66-77).
func TestPlayerAttackDropQueuedKeepsCurrentTarget(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	first, second := gatePlayerFake(2, 40, 500), gatePlayerFake(3, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	swingingAt(t, brain, strike, first)
	brain.Start(second, false)

	brain.DropQueued()
	strike.attackingNow = false
	brain.FinishedAttack()
	if strike.doAttackCalls != 2 || strike.target != first {
		t.Fatalf("swings = %d, last at %v, want the current target swung again", strike.doAttackCalls, strike.target)
	}
}

// A bow shot's end runs the attack queued behind it
// (PlayerAI.onEvtBowAttackReuse, PlayerAI.java:131-136).
func TestPlayerAttackThinkQueuedRunsTheQueuedTarget(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	first, second := gatePlayerFake(2, 40, 500), gatePlayerFake(3, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	swingingAt(t, brain, strike, first)
	brain.Start(second, false)

	strike.attackingNow = false
	brain.ThinkQueued()
	if strike.doAttackCalls != 2 || strike.target != second {
		t.Fatalf("swings = %d, last at %v, want the queued attack's swing", strike.doAttackCalls, strike.target)
	}
	if brain.ThinkQueued() || strike.doAttackCalls != 2 {
		t.Fatalf("second ThinkQueued() swung again (%d swings), want nothing queued", strike.doAttackCalls)
	}
}

// A cast queued behind the swing replaces the attack queued there, not the
// current one (PlayableAI.tryToCast, PlayableAI.java:312-318): the swing's
// target stays the current intention's final target until the swing ends,
// and dropping the queued cast leaves that attack going on.
func TestPlayerAttackQueueCastBehindSwingKeepsCurrentTarget(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	first, second := gatePlayerFake(2, 40, 500), gatePlayerFake(3, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	swingingAt(t, brain, strike, first)
	brain.Start(second, false)

	brain.QueueCastBehindSwing()
	if got := brain.Target(); got != first {
		t.Fatalf("Target() with a cast queued = %v, want the swing's target", got)
	}
	if brain.ThinkQueued() {
		t.Fatal("ThinkQueued() = true, want the queued attack replaced by the cast")
	}

	brain.DropQueued()
	strike.attackingNow = false
	brain.FinishedAttack()
	if strike.doAttackCalls != 2 || strike.target != first {
		t.Fatalf("swings = %d, last at %v, want the current target swung again", strike.doAttackCalls, strike.target)
	}
}

// An attack requested mid-cast waits on the cast as the next intention; a
// cast queued after it replaces it.
func TestPlayerAttackQueueCastReplacesAttackWaitingOnCast(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 500)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	pc.casting = true
	brain.Start(target, false)

	brain.QueueCastBehindSwing()
	if got := brain.Target(); got != nil {
		t.Fatalf("Target() = %v, want the waiting attack replaced", got)
	}
	pc.casting = false
	if resumed, _ := brain.ResumeAfterCast(false); resumed || strike.doAttackCalls != 0 {
		t.Fatalf("ResumeAfterCast() resumed = %v with %d swings, want nothing left to run", resumed, strike.doAttackCalls)
	}
}
