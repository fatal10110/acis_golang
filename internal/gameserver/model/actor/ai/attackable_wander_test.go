package ai

import (
	"testing"
)

func TestAttackableAIWanderReturnHome(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	owner.returnHome = true
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.SetWander()
	ai.Think()

	if owner.returnHomeCalls != 1 {
		t.Fatalf("ReturnHome calls = %d, want 1", owner.returnHomeCalls)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander while returning home", got)
	}
}

func TestAttackableAIWanderClearsWhenOutsideTerritoryAndNotReturning(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.SetWander()
	ai.Think()

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle outside territory without return home", got)
	}
}
