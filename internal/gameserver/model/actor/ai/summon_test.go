package ai

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

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

// TestSummonThinkLogsBothStopAndAttackBroadcastErrors is the regression
// test for the review finding that thinkAttackLocked's masking pattern
// (mirroring Attackable.thinkAttack) dropped attackErr from the log
// whenever both move.Stop's and attack.DoAttack's broadcasts failed in the
// same tick.
func TestSummonThinkLogsBothStopAndAttackBroadcastErrors(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	stopErr := errors.New("stop broadcast failed")
	attackErr := errors.New("attack broadcast failed")
	move := &summonMove{}
	move.stopErr = stopErr
	strike := &recordingAttack{canAttack: true, doAttackErr: attackErr}
	brain := NewSummon(owner, move, strike)
	var buf bytes.Buffer
	brain.SetLogger(zerolog.New(&buf))

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}

	logged := buf.String()
	if !strings.Contains(logged, stopErr.Error()) {
		t.Fatalf("logged error = %q, want it to contain stopErr (%v)", logged, stopErr)
	}
	if !strings.Contains(logged, attackErr.Error()) {
		t.Fatalf("logged error = %q, want it to contain attackErr (%v)", logged, attackErr)
	}
}

// TestSummonAITryToAttackContinuesAgainstFakeDeadTarget is the regression
// test for the review finding that targetLostLocked treated AlikeDead()
// (fake death included) as target loss, dropping the attack intention the
// moment a target entered fake death. AbstractAI.isTargetLost
// (AbstractAI.java:586-594) and SummonAI's override (SummonAI.java:275-281)
// have no death check at all — only a nil or no-longer-known target is
// "lost".
func TestSummonAITryToAttackContinuesAgainstFakeDeadTarget(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	target.alikeDead = true
	move := &summonMove{}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false against a fake-dead target, want the swing to proceed")
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want target despite fake death", strike.target)
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

func TestSummonAITryToCastApproachesBeforeCasting(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8})
	if cast.castCalled {
		t.Fatal("Cast() called while summon is closing distance")
	}
	if move.followTarget != target || move.followRange != 400 {
		t.Fatalf("offensive follow = (%v, %d), want (%v, 400)", move.followTarget, move.followRange, target)
	}
}

func TestSummonAITryToCastStopsFacesAndReportsFinalFailure(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	cast := &recordingCast{canAttempt: true, canCast: false, stopsMove: true}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8})
	if move.stopCount != 1 {
		t.Fatalf("Stop calls = %d, want 1", move.stopCount)
	}
	if owner.headingTarget != target {
		t.Fatalf("heading target = %v, want target", owner.headingTarget)
	}
	if owner.moveToPawnCalls != 1 || owner.moveToPawnTo != target {
		t.Fatalf("BroadcastMoveToPawn calls = (%d, %v), want (1, target)", owner.moveToPawnCalls, owner.moveToPawnTo)
	}
}

func TestSummonAITryToCastDropsBusyCastIntention(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{attackingNow: true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, &summonMove{}, strike)
	brain.SetCastController(cast)

	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}) {
		t.Fatal("TryToCast() = false, want queued cast intention")
	}
	strike.attackingNow = false
	cast.disabled = true
	brain.Think()
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle while cast controller is busy", got)
	}
}

func TestSummonAITryToAttackDoesNotWaitForDisabledSkills(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, &summonMove{}, strike)
	brain.SetCastController(&recordingCast{disabled: true})

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want attack accepted while skills are disabled")
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want target without a queued wait", strike.target)
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
