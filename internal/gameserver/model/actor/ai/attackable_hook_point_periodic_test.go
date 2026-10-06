package ai

import "testing"

// busyWithEmptyQueue leaves brain past its first periodic cycle with an
// unlatched attack on a target current and nothing queued: the next
// periodic cycle finds an actor that is still doing something and has no
// desire left.
func busyWithEmptyQueue(t *testing.T, brain *Attackable) {
	t.Helper()
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	attackUnlatched(t, brain, actor(2))
	brain.Desires().Clear()
}

// TestHookPointNoDesireTwiceOnPeriodicIdleOfABusyActor pins the periodic
// cycle that idles a busy actor whose queue is empty: desire selection
// idles it, which opens the no-desire point, and the cycle's own idle then
// runs again on the empty queue and opens it a second time, aborting again.
func TestHookPointNoDesireTwiceOnPeriodicIdleOfABusyActor(t *testing.T) {
	owner := actor(1)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	busyWithEmptyQueue(t, brain)
	owner.hooks.points = nil
	stops := strike.stopCalls
	var stopsAt []int
	owner.hooks.at = func(p HookPoint) {
		assertBrainUnlocked(t, brain, p)
		if p == HookNoDesire {
			stopsAt = append(stopsAt, strike.stopCalls-stops)
		}
	}

	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	assertHookPoints(t, owner, "on the periodic idle of a busy actor", HookSeeCreature, HookNoDesire, HookNoDesire)
	if len(stopsAt) != 2 || stopsAt[0] != 1 || stopsAt[1] != 2 {
		t.Fatalf("attack aborts seen at each no-desire point = %v, want [1 2]: each idle aborts first", stopsAt)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the cycle = %v, want idle", got)
	}
}

// TestHookPointNoDesireOnceWhenTheFirstQueuesADesire pins that the cycle's
// second idle waits on the queue: a no-desire hook that queues a desire
// keeps the point from opening again.
func TestHookPointNoDesireOnceWhenTheFirstQueuesADesire(t *testing.T) {
	owner := actor(1)
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
	busyWithEmptyQueue(t, brain)
	owner.hooks.points = nil
	owner.hooks.at = func(p HookPoint) {
		if p == HookNoDesire {
			brain.AddWanderDesire(5, 5)
		}
	}

	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	assertHookPoints(t, owner, "when the first no-desire queues a wander", HookSeeCreature, HookNoDesire)
}

// TestHookPointNoDesireOnceOnPeriodicIdleOfAnIdleOrStunnedActor pins the
// cycles that open the point once: an actor idle already is not idled by
// desire selection, and an out-of-control one selects nothing; only the
// cycle's own idle runs.
func TestHookPointNoDesireOnceOnPeriodicIdleOfAnIdleOrStunnedActor(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		owner := actor(1)
		brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
		if err := brain.TickThink(); err != nil {
			t.Fatalf("first TickThink() error: %v", err)
		}
		owner.hooks.points = nil
		if err := brain.TickThink(); err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
		assertHookPoints(t, owner, "on the periodic idle of an idle actor", HookSeeCreature, HookNoDesire)
	})
	t.Run("out of control", func(t *testing.T) {
		owner := actor(1)
		brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
		busyWithEmptyQueue(t, brain)
		owner.hooks.points = nil
		owner.confused = true
		if err := brain.TickThink(); err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
		assertHookPoints(t, owner, "on the periodic idle of an out-of-control actor", HookSeeCreature, HookNoDesire)
	})
}
