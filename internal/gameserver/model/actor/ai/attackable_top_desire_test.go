package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// topIs fails unless brain's top desire target is want (nil for none).
func topIs(t *testing.T, brain *Attackable, want attackable.Combatant, when string) {
	t.Helper()
	got := brain.TopDesireTarget()
	switch {
	case want == nil && got != nil:
		t.Fatalf("TopDesireTarget() %s = %d, want none", when, got.ObjectID())
	case want != nil && (got == nil || got.ObjectID() != want.ObjectID()):
		t.Fatalf("TopDesireTarget() %s = %v, want %d", when, got, want.ObjectID())
	}
}

// TestTopDesireTargetFollowsEveryPromotion pins NpcAI.runAI's
// _topDesireTarget: desire selection sets it to the final target of each
// desire it takes up, a follow and a flee included, and to none for a
// desire with no target.
func TestTopDesireTargetFollowsEveryPromotion(t *testing.T) {
	brain, _, _, _, _ := fleeSocialAI(t)
	leader, chaser := actor(2), actor(3)

	brain.AddFollowDesire(leader, 5)
	runAI(t, brain)
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want follow", got)
	}
	topIs(t, brain, leader, "after a follow was taken up")

	brain.AddFleeDesire(chaser, 300, 50)
	runAI(t, brain)
	if got := brain.CurrentIntention(); got != IntentionFlee {
		t.Fatalf("CurrentIntention() = %v, want flee", got)
	}
	topIs(t, brain, chaser, "after a flee was taken up")

	brain.Desires().Clear()
	brain.AddDoNothingDesire(40, 500)
	tickThink(t, brain)
	if got := brain.CurrentIntention(); got != IntentionNothing {
		t.Fatalf("CurrentIntention() = %v, want do nothing", got)
	}
	topIs(t, brain, nil, "after a desire with no target was taken up")
}

// TestTopDesireTargetOutlivesEventIdle pins where the top desire target is
// cleared: neither its desire leaving the queue nor the event idle clears
// it, the periodic cycle's empty-queue idle does. A script's write stands
// until then.
func TestTopDesireTargetOutlivesEventIdle(t *testing.T) {
	brain, _, _, _, _ := fleeSocialAI(t)
	leader, other := actor(2), actor(3)

	brain.AddFollowDesire(leader, 5)
	runAI(t, brain)
	brain.Desires().Clear()
	runAI(t, brain)
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the event idle = %v, want idle", got)
	}
	topIs(t, brain, leader, "after the event idle")

	brain.SetTopDesireTarget(other)
	runAI(t, brain)
	topIs(t, brain, other, "set by a script, after another event pass")

	tickThink(t, brain)
	topIs(t, brain, nil, "after the periodic cycle's idle")
}
