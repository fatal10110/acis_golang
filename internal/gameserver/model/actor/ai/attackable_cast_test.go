package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ---- from attackable_cast_test.go ----
func TestAttackableAIPromotesQueuedCastDesireAndCasts(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if got := ai.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionCast)
	}
	if !cast.castCalled || cast.castedTarget != target || cast.castedRef != ref {
		t.Fatalf("Cast call = (%v, %v, %v), want (true, target, %v)", cast.castCalled, cast.castedTarget, cast.castedRef, ref)
	}
	if move.followTarget != target || move.followRange != 400 {
		t.Fatalf("follow check = (%v, %d), want (%v, 400)", move.followTarget, move.followRange, target)
	}
}

func TestAttackableAICastStopsMovementAndFacesTargetForLongCast(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, stopsMove: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", move.stopCount)
	}
	if owner.headingTarget != target {
		t.Fatalf("heading target = %v, want target", owner.headingTarget)
	}
	if !cast.castCalled {
		t.Fatal("Cast() not called for a long-hit-time skill")
	}
}

func TestAttackableAICastDoesNotFaceSelfTarget(t *testing.T) {
	owner := actor(1)
	// A creature's own region always contains itself, so it always "knows"
	// itself; the fake's known map mirrors that explicitly here since it
	// otherwise only tracks other actors.
	owner.known[owner.ObjectID()] = true
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, stopsMove: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: owner, Skill: ref, Weight: 10})
	ai.RunAI()

	if owner.headingTarget != nil {
		t.Fatalf("heading target = %v, want none for self-targeted skill", owner.headingTarget)
	}
	if !cast.castCalled {
		t.Fatal("Cast() not called for a self-targeted skill")
	}
}

func TestAttackableAICastStartsOffensiveFollowBeforeCasting(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{followStarted: true}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if cast.castCalled {
		t.Fatal("Cast() called while still closing distance")
	}
	if move.followTarget != target || move.followRange != 400 {
		t.Fatalf("follow check = (%v, %d), want (%v, 400)", move.followTarget, move.followRange, target)
	}
	if owner.runStanceCalls != 1 {
		t.Fatalf("run stance calls = %d, want 1 (cast-approach follow switches to run)", owner.runStanceCalls)
	}
}

func TestAttackableAICastRespectsPreMovementCooldownGate(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: false, canCast: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none while skill is on cooldown", move.followTarget)
	}
	if cast.castCalled {
		t.Fatal("Cast() called while skill is on cooldown")
	}
}

func TestAttackableAICastRespectsFinalCastGate(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: false}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if cast.castCalled {
		t.Fatal("Cast() called after the final cast gate rejected the attempt")
	}
	if owner.moveToPawnCalls != 1 || owner.moveToPawnTo != target {
		t.Fatalf("BroadcastMoveToPawn calls = (%d, %v), want (1, target)", owner.moveToPawnCalls, owner.moveToPawnTo)
	}
}

// TestAttackableRunAICastStopsMovementAndStillFacesTarget covers the
// rejected-cast path: a cast whose skill freezes the caster cancels the walk,
// and the rotation-only notice observers need still goes out on the same tick
// even though the cast itself was rejected.
func TestAttackableRunAICastStopsMovementAndStillFacesTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: false, stopsMove: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error = %v, want nil", err)
	}

	if move.stopCount != 1 {
		t.Fatalf("move.Stop calls = %d, want 1", move.stopCount)
	}
	if owner.moveToPawnCalls != 1 || owner.moveToPawnTo != target {
		t.Fatalf("BroadcastMoveToPawn calls = (%d, %v), want (1, target)", owner.moveToPawnCalls, owner.moveToPawnTo)
	}
}

func TestAttackableAICastFinalGateRejectDoesNotBroadcastForSelfTarget(t *testing.T) {
	owner := actor(1)
	owner.known[owner.ObjectID()] = true
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: false}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: owner, Skill: ref, Weight: 10})
	ai.RunAI()

	if owner.moveToPawnCalls != 0 {
		t.Fatalf("BroadcastMoveToPawn calls = %d, want 0 for a self-targeted skill", owner.moveToPawnCalls)
	}
}

func TestAttackableAICastSummonFriendBypassesTargetLostCheck(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): false}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, skillType: "SUMMON_FRIEND"}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if !cast.castCalled || cast.castedTarget != target {
		t.Fatal("Cast() not called for a SUMMON_FRIEND cast against an unknown target")
	}
}

func TestAttackableAIIgnoresCastDesireForLostTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): false}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if cast.castCalled {
		t.Fatal("Cast() called for a lost target")
	}
}

func TestAttackableAICastNoOpsWithoutCastController(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	ai := NewAttackable(owner, move, &recordingAttack{})

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})

	ai.RunAI() // must not panic with no CastController wired.

	if got := ai.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionCast)
	}
}
