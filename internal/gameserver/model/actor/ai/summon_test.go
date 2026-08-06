package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestSummonAITryToAttackExecutesPhysicalAttack(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want target", strike.target)
	}
	if move.followTarget != target || move.followRange != owner.attackRange {
		t.Fatalf("offensive follow = (%v, %d), want (%v, %d)", move.followTarget, move.followRange, target, owner.attackRange)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack", got)
	}
}

func TestSummonAITryToAttackQueuesWhileSwingingAndExecutesOnThink(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	brain := NewSummon(owner, &summonMove{}, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want queued attack accepted while already attacking")
	}
	if strike.target != nil {
		t.Fatalf("attack target = %v while busy, want queued without a new swing", strike.target)
	}
	if kind, queuedTarget, ok := brain.NextIntention(); !ok || kind != IntentionAttack || queuedTarget != target {
		t.Fatalf("NextIntention() = (%v,%v,%v), want attack,target,true", kind, queuedTarget, ok)
	}

	strike.attackingNow = false
	brain.Think()
	if strike.target != target {
		t.Fatalf("attack target after Think = %v, want target", strike.target)
	}
}

func TestSummonAIThinkPreservesQueuedRetargetWhileCurrentAttackIsBusy(t *testing.T) {
	owner := actor(100)
	firstTarget := actor(200)
	secondTarget := actor(300)
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, &summonMove{}, strike)

	if !brain.TryToAttack(firstTarget) {
		t.Fatal("TryToAttack(first) = false, want accepted attack")
	}
	if strike.target != firstTarget {
		t.Fatalf("attack target = %v, want first target", strike.target)
	}

	strike.attackingNow = true
	if !brain.TryToAttack(secondTarget) {
		t.Fatal("TryToAttack(second) = false, want queued retarget accepted while busy")
	}
	brain.Think()

	if kind, queuedTarget, ok := brain.NextIntention(); !ok || kind != IntentionAttack || queuedTarget != secondTarget {
		t.Fatalf("NextIntention() = (%v,%v,%v), want attack,second target,true", kind, queuedTarget, ok)
	}
}

func TestSummonAITryToFollowStartsFriendlyFollow(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	brain := NewSummon(owner, move, &recordingAttack{})

	if !brain.TryToFollow(target) {
		t.Fatal("TryToFollow() = false, want accepted follow")
	}
	if move.friendlyTarget != target || move.friendlyRange != 70 {
		t.Fatalf("friendly follow = (%v, %d), want (%v, 70)", move.friendlyTarget, move.friendlyRange, target)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want follow", got)
	}
}

func TestSummonAITryToCastExecutesImmediately(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)
	ref := skill.Ref{ID: 4139, Level: 8}

	if !brain.TryToCast(target, ref) {
		t.Fatal("TryToCast() = false, want accepted cast")
	}
	if cast.castedTarget != target || cast.castedRef != ref {
		t.Fatalf("cast = (%v,%v), want (%v,%v)", cast.castedTarget, cast.castedRef, target, ref)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle once the cast is dispatched", got)
	}
}

func TestSummonAITryToCastQueuesWhileBusyAndExecutesOnThink(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, &summonMove{}, strike)
	brain.SetCastController(cast)
	ref := skill.Ref{ID: 4139, Level: 8}

	if !brain.TryToCast(target, ref) {
		t.Fatal("TryToCast() = false, want queued cast accepted while attacking")
	}
	if cast.castedTarget != nil {
		t.Fatalf("cast target = %v while busy, want no cast dispatched yet", cast.castedTarget)
	}
	if kind, queuedTarget, ok := brain.NextIntention(); !ok || kind != IntentionCast || queuedTarget != target {
		t.Fatalf("NextIntention() = (%v,%v,%v), want cast,target,true", kind, queuedTarget, ok)
	}

	strike.attackingNow = false
	brain.Think()
	if cast.castedTarget != target {
		t.Fatalf("cast target after Think = %v, want target", cast.castedTarget)
	}
}

func TestSummonAITryToCastRejectsWithoutCastController(t *testing.T) {
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})

	if brain.TryToCast(actor(200), skill.Ref{ID: 4139, Level: 8}) {
		t.Fatal("TryToCast() = true with no CastController attached, want false")
	}
}

func TestSummonAITryToCastRejectsWhenCanAttemptFails(t *testing.T) {
	target := actor(200)
	cast := &recordingCast{canAttempt: false, canCast: true}
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}) {
		t.Fatal("TryToCast() = true when CanAttempt rejects the skill, want false")
	}
	if cast.castedTarget != nil {
		t.Fatalf("cast target = %v, want no cast dispatched", cast.castedTarget)
	}
}

func TestSummonAITryToIdleStopsMovement(t *testing.T) {
	move := &summonMove{}
	brain := NewSummon(actor(100), move, &recordingAttack{})

	brain.TryToIdle()

	if move.stopCount != 1 {
		t.Fatalf("Stop calls = %d, want 1", move.stopCount)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
}

type summonMove struct {
	recordingMove
	friendlyTarget attackable.Combatant
	friendlyRange  int
}

func (m *summonMove) MaybeStartFriendlyFollow(target attackable.Combatant, offset int) (bool, error) {
	m.friendlyTarget = target
	m.friendlyRange = offset
	return true, nil
}
