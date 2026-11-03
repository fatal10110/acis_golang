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

// TestTopDesireTargetClearedByEveryCycleIdle pins NpcAI.runAI's cycle idle
// clearing the top desire target whatever defers or holds it: the idle
// deferred past a latched attack, in control or out of it, and the idle
// that runs while a social animation holds desire selection.
func TestTopDesireTargetClearedByEveryCycleIdle(t *testing.T) {
	for _, row := range []struct {
		name   string
		before func(*Attackable, *fakeActor)
	}{
		{"latched attack, in control", nil},
		{"latched attack, out of control", func(_ *Attackable, owner *fakeActor) { owner.denyAction = true }},
	} {
		t.Run(row.name, func(t *testing.T) {
			brain, owner, _ := latchedAttackAI(t)
			if !brain.hasLatch() {
				t.Fatal("setup: no latched attack")
			}
			topIs(t, brain, actor(2), "after the attack was taken up")
			if row.before != nil {
				row.before(brain, owner)
			}
			tickThink(t, brain)
			topIs(t, brain, nil, "after the cycle's idle past the latch")
		})
	}

	t.Run("held by a social animation", func(t *testing.T) {
		brain, _, _, _, _ := fleeSocialAI(t)
		other := actor(3)
		brain.AddSocialDesire(3, 2000, 50)
		runAI(t, brain)
		if !brain.selectionHeld() {
			t.Fatal("setup: the social animation does not hold selection")
		}
		brain.SetTopDesireTarget(other)
		runAI(t, brain)
		topIs(t, brain, other, "after an event pass while held")
		tickThink(t, brain)
		topIs(t, brain, nil, "after the cycle's idle while held")
	})
}
