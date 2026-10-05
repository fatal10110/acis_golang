package ai

import "testing"

// TestHookPointNoDesireOnIdleAttackFinished pins the finished attack's think
// on an in-control actor that desire selection leaves idle: the idle step
// runs again, the no-desire point opens after the abort and the walk stance
// with the brain mutex released, and no idle follow or wander is queued.
func TestHookPointNoDesireOnIdleAttackFinished(t *testing.T) {
	owner := actor(1)
	owner.idleWander = true
	move := &recordingMove{}
	strike := &recordingAttack{}
	brain := NewAttackable(owner, move, strike)
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	brain.Desires().Clear()
	owner.hooks.points = nil
	stops, stances := strike.stopCalls, owner.walkStanceCalls
	owner.hooks.at = func(p HookPoint) {
		assertBrainUnlocked(t, brain, p)
		if strike.stopCalls == stops || owner.walkStanceCalls == stances {
			t.Fatalf("no-desire point before the idle abort: attack stops %d -> %d, walk stance %d -> %d",
				stops, strike.stopCalls, stances, owner.walkStanceCalls)
		}
	}

	if err := brain.AttackFinished(); err != nil {
		t.Fatalf("AttackFinished() error: %v", err)
	}
	assertHookPoints(t, owner, "on an idle actor's swing end", HookNoDesire)
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the swing end = %v, want idle", got)
	}
	if got := brain.Desires().Len(); got != 0 {
		t.Fatalf("queued desires after the swing end = %d, want none", got)
	}
}

// TestHookPointNoDesireTwiceOnAttackFinishedEventIdle pins the swing end
// whose desire selection idles an attack with an empty queue: the
// selection's idle opens the no-desire point, and the finished attack's
// think on the now idle actor opens it again.
func TestHookPointNoDesireTwiceOnAttackFinishedEventIdle(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	attackUnlatched(t, brain, target)
	brain.Desires().Clear()
	owner.hooks.points = nil
	owner.hooks.at = func(p HookPoint) { assertBrainUnlocked(t, brain, p) }

	if err := brain.AttackFinished(); err != nil {
		t.Fatalf("AttackFinished() error: %v", err)
	}
	assertHookPoints(t, owner, "on a swing end idling the attack", HookNoDesire, HookNoDesire)
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the swing end = %v, want idle", got)
	}
}

// TestHookPointNoDesireNotOnAttackFinishedDroppedToIdle pins that a swing
// end before the first periodic cycle, which drops an attack whose desire
// left the queue without the empty-queue idle, takes no idle step: the
// actor was not idle and nothing idled it, as the reference's attack would
// still be current there and its think would step the attack.
func TestHookPointNoDesireNotOnAttackFinishedDroppedToIdle(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	attackUnlatched(t, brain, target)
	brain.Desires().Clear()
	owner.hooks.points = nil
	stops := strike.stopCalls

	if err := brain.AttackFinished(); err != nil {
		t.Fatalf("AttackFinished() error: %v", err)
	}
	assertHookPoints(t, owner, "on a swing end dropping the attack")
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the swing end = %v, want idle", got)
	}
	if strike.stopCalls != stops || owner.walkStanceCalls != 0 {
		t.Fatalf("idle step ran on the dropped attack: attack stops %d -> %d, walk stance %d",
			stops, strike.stopCalls, owner.walkStanceCalls)
	}
}

// attackUnlatched makes an attack on target current with no latch left, so
// desire selection idles it once its desire leaves the queue: the first
// attack desire latches, and the second selection, coming from an attack,
// clears the latch.
func attackUnlatched(t *testing.T, brain *Attackable, target *fakeActor) {
	t.Helper()
	brain.AddAttackDesire(target, 100)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("setup RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack || brain.hasLatch() {
		t.Fatalf("setup: CurrentIntention() = %v latched %v, want an unlatched %v",
			got, brain.hasLatch(), IntentionAttack)
	}
}
