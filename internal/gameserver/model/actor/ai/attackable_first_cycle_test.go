package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recordingRoute counts the route walks the AI asks for.
type recordingRoute struct{ walks, leaves int }

func (r *recordingRoute) Walk(string) { r.walks++ }
func (r *recordingRoute) Leave()      { r.leaves++ }

// TestEventSelectionWaitsForFirstCycle pins the first-cycle gate on an
// event pass: before the first periodic cycle, RunAI takes up no queued
// walk; the first cycle, which counts itself only after its selection,
// takes up nothing either; the next event takes the walk up.
func TestEventSelectionWaitsForFirstCycle(t *testing.T) {
	owner := actor(1)
	owner.x = 100
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	dest := location.Location{X: 300}
	brain.AddMoveToDesire(dest, 1_000)

	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle || move.home != (location.Location{}) {
		t.Fatalf("before the first cycle: intention %v, walk %v; want idle and no walk", got, move.home)
	}
	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("intention after the first cycle = %v, want idle", got)
	}
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() after the first cycle error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo || move.home != dest {
		t.Fatalf("after the first cycle: intention %v, walk %v; want move_to toward %v", got, move.home, dest)
	}
}

// castBeforeFirstCycle takes up a cast before the brain's first periodic
// cycle through a lighter queued attack desire (instantRun), queues a walk
// lighter than the cast, then drops the attack desire and the cast's own
// desire, as a cast end does.
func castBeforeFirstCycle(t *testing.T) (*Attackable, *recordingMove, *recordingCast) {
	t.Helper()
	owner, target, attacker := actor(1), actor(2), actor(3)
	owner.x = 100
	move := &recordingMove{}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewAttackable(owner, move, &recordingAttack{})
	brain.SetCastController(cast)
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: attacker, Weight: 1})
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 10})
	brain.AddMoveToDesire(location.Location{X: 300}, 5)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionCast || cast.castCalls != 1 {
		t.Fatalf("setup: intention %v, casts %d; want the cast taken up on the attack desire", got, cast.castCalls)
	}
	brain.Desires().Remove(IntentionAttack, attacker)
	brain.ClearCurrentDesire()
	return brain, move, cast
}

// TestCastEndBeforeFirstCycleTakesUpNoQueuedWalk pins the gate on a cast
// end's selection: with the attack desire gone, nothing opens it before the
// first periodic cycle, so the queued walk waits and the cast stays current.
func TestCastEndBeforeFirstCycleTakesUpNoQueuedWalk(t *testing.T) {
	brain, move, cast := castBeforeFirstCycle(t)

	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() on the cast end error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("intention after the cast end = %v, want the cast kept", got)
	}
	if move.home != (location.Location{}) || cast.castCalls != 1 {
		t.Fatalf("after the cast end: walk %v, casts %d; want no walk and no new cast", move.home, cast.castCalls)
	}
	if !brain.Desires().Has(&Desire{Kind: IntentionMoveTo, Location: location.Location{X: 300}}) {
		t.Fatal("walk desire gone after the cast end, want it still queued")
	}
}

// TestFinishedSwingBeforeFirstCycleStepsCurrentIntention pins the finished
// swing's own think: its selection takes up no queued walk before the first
// periodic cycle, but the think still steps the current cast.
func TestFinishedSwingBeforeFirstCycleStepsCurrentIntention(t *testing.T) {
	brain, move, cast := castBeforeFirstCycle(t)

	if err := brain.AttackFinished(); err != nil {
		t.Fatalf("AttackFinished() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionCast || move.home != (location.Location{}) {
		t.Fatalf("after the finished swing: intention %v, walk %v; want the cast kept and no walk", got, move.home)
	}
	if cast.castCalls != 2 {
		t.Fatalf("casts after the finished swing = %d, want the think's cast step", cast.castCalls)
	}
}

// TestFinishedSwingTakesNoRouteStep pins that the finished swing's think
// has no route-walk step: with nothing promoted, a current route walk is
// not walked again.
func TestFinishedSwingTakesNoRouteStep(t *testing.T) {
	owner, attacker := actor(1), actor(3)
	route := &recordingRoute{}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.SetRoute(route)
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: attacker, Weight: 1})
	brain.AddMoveRouteDesire("route", 10)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveRoute || route.walks != 1 {
		t.Fatalf("setup: intention %v, route walks %d; want the route taken up once", got, route.walks)
	}
	brain.Desires().Remove(IntentionAttack, attacker)

	if err := brain.AttackFinished(); err != nil {
		t.Fatalf("AttackFinished() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveRoute || route.walks != 1 {
		t.Fatalf("after the finished swing: intention %v, route walks %d; want the route kept, not walked again", got, route.walks)
	}
}
