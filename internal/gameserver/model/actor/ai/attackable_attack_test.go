package ai

import (
	"testing"
)

func TestAttackableAIAddDamageHateDoesNotQueueAttackDesire(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(target, 7, 30)

	if got := ai.Threats().Hate(target); got != 30 {
		t.Fatalf("hate = %v, want 30", got)
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
}

// TestAttackableAIAddAttackDesireFeedsThreatTable pins #2340: Java's
// NpcAI.addAttackDesire is the single choke point that both queues the
// Desire and, under its updateAggro=true default, writes the same weight
// into the AggroList/threat table (NpcAI.java:713-727). Every Go call site
// currently uses that default, so AddAttackDesire alone must raise the
// target's threat hate, not just queue a Desire.
func TestAttackableAIAddAttackDesireFeedsThreatTable(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddAttackDesire(target, 200)

	if got := ai.Threats().Hate(target); got != 200 {
		t.Fatalf("hate = %v, want 200", got)
	}
	if threat, ok := ai.Threats().Get(target); !ok || threat.Damage != 0 {
		t.Fatalf("threat = (%+v, %v), want damage 0 (weight-only convenience call)", threat, ok)
	}
}

// TestAttackableAICombatDamageHateUsesCallerWeightNotDamage pins #2340:
// Npc.reduceCurrentHp's own addDamageHate(attacker, damage, 0) call never
// raises hate (Npc.java:395); real hate comes only from the ATTACKED-event
// attack Desire queued alongside it, at whatever weight the caller derived
// (the per-script onAttacked formula — see Hostile.attackedHateWeight, which
// this generic AI layer does not know about). AddCombatDamageHate's
// resulting threat hate must equal that caller-supplied weight, not the raw
// damage passed for the threat table's damage bookkeeping.
func TestAttackableAICombatDamageHateUsesCallerWeightNotDamage(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddCombatDamageHate(target, 9999, 42)

	if got := ai.Threats().Hate(target); got != 42 {
		t.Fatalf("hate = %v, want caller-supplied weight 42, not raw damage", got)
	}
	if threat, ok := ai.Threats().Get(target); !ok || threat.Damage != 9999 {
		t.Fatalf("threat = (%+v, %v), want damage 9999 preserved", threat, ok)
	}
}

func TestAttackableAIChoosesMostHatedTargetToAttack(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	owner.known = map[int32]bool{low.ObjectID(): true, high.ObjectID(): true}
	owner.attackRange = 40
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)
	ai.RunAI()

	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionAttack)
	}
	if strike.target != high {
		t.Fatalf("attacked target = %v, want high threat target", strike.target)
	}
	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", move.stopCount)
	}
	if move.followTarget != high || move.followRange != 40 {
		t.Fatalf("follow check = (%v, %d), want (%v, 40)", move.followTarget, move.followRange, high)
	}
}

// TestAttackableRunAIStopsMovementAndAttacksOnTheSameTick pins thinkAttack's
// two-call shape: an accepted swing cancels the walk and starts the attack in
// the same tick, and reports no error for either.
func TestAttackableRunAIStopsMovementAndAttacksOnTheSameTick(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 10)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error = %v, want nil", err)
	}

	if move.stopCount != 1 {
		t.Fatalf("move.Stop calls = %d, want 1", move.stopCount)
	}
	if strike.doAttackCalls != 1 || strike.target != target {
		t.Fatalf("DoAttack calls = (%d, %v), want (1, target)", strike.doAttackCalls, strike.target)
	}
}

func TestAttackableAIStartsOffensiveFollowBeforeAttack(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.attackRange = 80
	move := &recordingMove{followStarted: true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 100)
	ai.RunAI()

	if move.followTarget != target || move.followRange != 80 {
		t.Fatalf("follow check = (%v, %d), want (%v, 80)", move.followTarget, move.followRange, target)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while follow starts", strike.target)
	}
	if move.stopCount != 0 {
		t.Fatalf("stop count = %d, want 0 while follow starts", move.stopCount)
	}
}

func TestAttackableAIQueuesAttackWhileBusy(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 100)
	ai.RunAI()

	next, nextTarget, ok := ai.NextIntention()
	if !ok {
		t.Fatal("NextIntention() ok = false, want true")
	}
	if next != IntentionAttack || nextTarget != target {
		t.Fatalf("NextIntention() = (%v, %v), want (%v, target)", next, nextTarget, IntentionAttack)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while already attacking", strike.target)
	}
}

func TestAttackableAIIgnoresLostTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): false}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 100)
	ai.RunAI()

	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none for lost target", move.followTarget)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none for lost target", strike.target)
	}
}

// TestAttackableAIKeepsTargetWhenCanAttackFails is the regression test for
// PR #936's skipAttackTarget: on a CanAttack failure the reference
// (CreatureAI.thinkAttack, `if (!_actor.getAttack().canAttack(target)) return;`)
// leaves the current target, its hate and its ATTACK desire untouched and
// retries next tick. The removed skipAttackTarget instead zeroed the
// blocked target's hate, dropped its desire, and transferred the hate to
// the next most-hated attacker with no validity filter.
func TestAttackableAIKeepsTargetWhenCanAttackFails(t *testing.T) {
	owner := actor(1)
	blocked := actor(2)
	other := actor(3)
	owner.known = map[int32]bool{blocked.ObjectID(): true, other.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{
		canAttackTarget: map[int32]bool{
			blocked.ObjectID(): false,
			other.ObjectID():   true,
		},
	}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, other, 0, 25)
	addAttackHate(ai, blocked, 0, 100)
	ai.RunAI()

	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while blocked target is retried", strike.target)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v (kept committed to blocked target)", got, IntentionAttack)
	}
	if got := ai.Threats().Hate(blocked); got != 100 {
		t.Fatalf("blocked target hate = %v, want untouched 100", got)
	}
	if got := ai.Threats().Hate(other); got != 25 {
		t.Fatalf("other target hate = %v, want untouched 25 (no hate transfer)", got)
	}
}
