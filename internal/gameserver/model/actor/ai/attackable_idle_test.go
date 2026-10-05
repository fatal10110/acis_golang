package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestAttackableAIIdleQueuesWanderAndWalks(t *testing.T) {
	owner := actor(1)
	owner.idleWander = true
	owner.moveSpeed = 40
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after idle abort = %v, want idle", got)
	}
	got, ok := ai.Desires().Peek()
	if !ok || got.Kind != IntentionWander || got.Timer != 5 || got.Weight != 5 {
		t.Fatalf("queued wander = (%v %+v), want timer 5 weight 5", ok, got)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander move = %d after queue tick, want 0 (promote next cycle)", owner.wanderCalls)
	}

	if err := ai.TickThink(); err != nil {
		t.Fatalf("promote TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}
	if owner.wanderCalls != 1 || owner.wanderOffset != 120 {
		t.Fatalf("wander move = %d offset %d, want 1 call offset 120", owner.wanderCalls, owner.wanderOffset)
	}
}

func TestAttackableAIFirstTickDoesNotPromoteWander(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 40
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after first TickThink = %v, want idle", got)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander move = %d on first TickThink, want 0", owner.wanderCalls)
	}
	if !ai.Desires().Has(&Desire{Kind: IntentionWander}) {
		t.Fatal("wander desire dropped on first TickThink, want it kept")
	}

	if err := ai.TickThink(); err != nil {
		t.Fatalf("second TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after second TickThink = %v, want wander", got)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander move = %d after second TickThink, want 1", owner.wanderCalls)
	}
}

func TestAttackableAIFirstTickPromotesWhenAttackQueued(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	ai.Threats().AddDamage(target, 0, 10)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after first TickThink = %v, want attack", got)
	}
	if strike.target != target {
		t.Fatalf("attacked target = %v, want queued attacker", strike.target)
	}
}

func TestAttackableAIFirstTickPromotesHighestWeightWhenAttackOpensGate(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.moveSpeed = 40
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	ai.Threats().AddDamage(target, 0, 1)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 1})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 100})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander (highest weight, gate opened by queued attack)", got)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while wander outranks attack", strike.target)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander move = %d, want 1", owner.wanderCalls)
	}
}

func TestAttackableAIFirstTickPromotesAfterAttackDesirePruned(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.moveSpeed = 40
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander (ATTACK presence latched before prune)", got)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander move = %d, want 1", owner.wanderCalls)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("ATTACK desire still queued after empty-threat prune")
	}
}

func TestAttackableAIDoesNotPromoteWhileCasting(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 40
	cast := &recordingCast{casting: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.lifeTime.Store(1)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() while casting = %v, want idle", got)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander move = %d while casting, want 0", owner.wanderCalls)
	}
}

func TestAttackableAIIdleHoldPositionForcesWalkStance(t *testing.T) {
	owner := actor(1)
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{})

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle (no wander queue)", got)
	}
	if owner.walkStanceCalls != 1 {
		t.Fatalf("walk stance calls = %d, want 1", owner.walkStanceCalls)
	}
	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", move.stopCount)
	}
}

func TestAttackableAIIdleAbortsInFlightAttackWhenQueueEmpty(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	ai := NewAttackable(owner, move, strike)
	addAttackHate(ai, target, 0, 20)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack", got)
	}

	ai.Desires().Clear()
	strike.attackingNow = true
	if err := ai.Think(); err != nil {
		t.Fatalf("empty-queue Think() error: %v", err)
	}
	if strike.stopCalls != 0 {
		t.Fatalf("attack Stop calls = %d after event Think, want 0", strike.stopCalls)
	}

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}

	if strike.stopCalls != 1 {
		t.Fatalf("attack Stop calls = %d, want 1", strike.stopCalls)
	}
	if owner.walkStanceCalls != 1 {
		t.Fatalf("walk stance calls = %d, want 1", owner.walkStanceCalls)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle after empty-queue abort", got)
	}
}

func TestAttackableAIArrivedThinkDoesNotAbortInFlightAttack(t *testing.T) {
	owner := actor(1)
	move := &recordingMove{}
	strike := &recordingAttack{attackingNow: true}
	ai := NewAttackable(owner, move, strike)
	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	ai.Arrived()
	stops := strike.stopCalls
	walk := owner.walkStanceCalls
	if err := ai.Think(); err != nil {
		t.Fatalf("arrival Think() error: %v", err)
	}
	if strike.stopCalls != stops {
		t.Fatalf("attack Stop calls = %d on arrival Think, want %d", strike.stopCalls, stops)
	}
	// The finished wander is still current, but THINK has no wander step.
	if owner.walkStanceCalls != walk {
		t.Fatalf("walk stance calls = %d on arrival Think, want %d", owner.walkStanceCalls, walk)
	}

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if strike.stopCalls != stops+1 {
		t.Fatalf("attack Stop calls = %d after TickThink, want %d", strike.stopCalls, stops+1)
	}
}

func TestAttackableAIIdleSkipsAbortWhileCasting(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	cast := &recordingCast{canAttempt: true, canCast: true, casting: true}
	ai := NewAttackable(owner, move, strike)
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 10})
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}

	ai.Desires().Clear()
	cast.casting = true
	walkBefore := owner.walkStanceCalls
	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("casting TickThink() error: %v", err)
	}
	if strike.stopCalls != 0 {
		t.Fatalf("attack Stop calls = %d, want 0 while casting", strike.stopCalls)
	}
	if cast.stopCalls != 0 {
		t.Fatalf("cast Stop calls = %d, want 0 while casting", cast.stopCalls)
	}
	if owner.walkStanceCalls != walkBefore {
		t.Fatalf("walk stance calls = %d, want %d while casting", owner.walkStanceCalls, walkBefore)
	}
}
