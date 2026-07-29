package ai

import (
	"testing"
)

func TestAttackableAIChoosesMostHatedTargetToAttack(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	owner.known = map[int32]bool{low.ObjectID(): true, high.ObjectID(): true}
	owner.attackRange = 40
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	ai.AddDamageHate(low, 0, 10)
	ai.AddDamageHate(high, 0, 25)
	ai.Think()

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

func TestAttackableAIStartsOffensiveFollowBeforeAttack(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.attackRange = 80
	move := &recordingMove{followStarted: true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	ai.AddDamageHate(target, 0, 100)
	ai.Think()

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

	ai.AddDamageHate(target, 0, 100)
	ai.Think()

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

	ai.AddDamageHate(target, 0, 100)
	ai.Think()

	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none for lost target", move.followTarget)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none for lost target", strike.target)
	}
}

func TestAttackableAIReselectsWhenTopAttackTargetCannotBeUsed(t *testing.T) {
	owner := actor(1)
	blocked := actor(2)
	reachable := actor(3)
	owner.known = map[int32]bool{blocked.ObjectID(): true, reachable.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{
		canAttackTarget: map[int32]bool{
			blocked.ObjectID():   false,
			reachable.ObjectID(): true,
		},
	}
	ai := NewAttackable(owner, move, strike)

	ai.AddDamageHate(reachable, 0, 25)
	ai.AddDamageHate(blocked, 0, 100)
	ai.Think()

	if strike.target != reachable {
		t.Fatalf("attacked target = %v, want reachable fallback target", strike.target)
	}
	if got := ai.Threats().Hate(blocked); got != 0 {
		t.Fatalf("blocked target hate = %v, want stopped", got)
	}
	if got := ai.Threats().Hate(reachable); got <= 25 {
		t.Fatalf("reachable target hate = %v, want reselection hate transferred above original 25", got)
	}
}
