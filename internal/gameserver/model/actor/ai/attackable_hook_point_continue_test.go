package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestHookPointNoDesireOnIdleContinue pins the no-desire point on the
// continue pass of an idle actor, raised when a control effect or a bow's
// reuse ends: the idle step runs again, the point opens after the abort and
// the walk stance with the brain mutex released, and no idle follow or
// wander is queued.
func TestHookPointNoDesireOnIdleContinue(t *testing.T) {
	owner := actor(1)
	owner.idleWander = true
	move := &recordingMove{}
	strike := &recordingAttack{}
	brain := NewAttackable(owner, move, strike)
	owner.hooks.at = func(p HookPoint) {
		assertBrainUnlocked(t, brain, p)
		if move.stopCount == 0 || strike.stopCalls == 0 || owner.walkStanceCalls == 0 {
			t.Fatalf("no-desire point before the idle abort: move stops %d, attack stops %d, walk stance %d",
				move.stopCount, strike.stopCalls, owner.walkStanceCalls)
		}
	}

	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertHookPoints(t, owner, "on an idle continue pass", HookNoDesire)
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the continue pass = %v, want idle", got)
	}
	if got := brain.Desires().Len(); got != 0 {
		t.Fatalf("queued desires after the continue pass = %d, want none", got)
	}

	// An out-of-control actor whose swing finished takes the same continue
	// pass.
	owner.denyAction = true
	owner.hooks.points = nil
	if err := brain.AttackFinished(); err != nil {
		t.Fatalf("AttackFinished() error: %v", err)
	}
	assertHookPoints(t, owner, "on an out-of-control idle swing end", HookNoDesire)
}

// TestHookPointNoDesireNotOnContinueDroppedToIdle pins that a continue pass
// on an attack whose desire left the queue opens no no-desire point: the
// actor was not idle when the pass started, so it takes no idle step.
func TestHookPointNoDesireNotOnContinueDroppedToIdle(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	brain.AddAttackDesire(target, 100)
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("setup: CurrentIntention() = %v, want %v", got, IntentionAttack)
	}
	brain.Desires().Clear()
	owner.hooks.points = nil
	stops := strike.stopCalls

	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertHookPoints(t, owner, "on a continue pass dropping the attack")
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the continue pass = %v, want idle", got)
	}
	if strike.stopCalls != stops || owner.walkStanceCalls != 0 {
		t.Fatalf("idle step ran on the dropped attack: attack stops %d -> %d, walk stance %d",
			stops, strike.stopCalls, owner.walkStanceCalls)
	}
}

// TestHookPointContinueOnBusyIntentionOpensNoPoint pins that the continue
// pass of a walk under way opens no point.
func TestHookPointContinueOnBusyIntentionOpensNoPoint(t *testing.T) {
	owner := actor(1)
	owner.x = 100
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.AddMoveToDesire(location.Location{X: 300}, 1_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	owner.hooks.points = nil

	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertHookPoints(t, owner, "on a walk's continue pass")
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the continue pass = %v, want %v", got, IntentionMoveTo)
	}
}
