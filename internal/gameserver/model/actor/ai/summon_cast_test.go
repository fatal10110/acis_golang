package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestSummonAITryToCastExecutesImmediately(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)
	ref := skill.Ref{ID: 4139, Level: 8}

	if !brain.TryToCast(target, ref, false) {
		t.Fatal("TryToCast() = false, want accepted cast")
	}
	if cast.castedTarget != target || cast.castedRef != ref {
		t.Fatalf("cast = (%v,%v), want (%v,%v)", cast.castedTarget, cast.castedRef, target, ref)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle once the cast is dispatched", got)
	}
}

func TestSummonAITryToCastAimsAtTheFinalTarget(t *testing.T) {
	owner := actor(100)
	clicked := actor(200)
	cast := &recordingCast{canAttempt: true, canCast: true, final: owner}
	brain := NewSummon(owner, &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if !brain.TryToCast(clicked, skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = false, want accepted cast")
	}
	if cast.castedTarget != owner {
		t.Fatalf("cast target = %v, want the final target %v", cast.castedTarget, owner)
	}
}

func TestSummonAITryToCastWithoutFinalTargetIsDropped(t *testing.T) {
	cast := &recordingCast{canAttempt: true, canCast: true, noFinal: true}
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if brain.TryToCast(actor(200), skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = true with no final target, want the request dropped")
	}
	if cast.castCalled || brain.CurrentIntention() != IntentionIdle {
		t.Fatalf("dropped request cast = %v, intention = %v; want no cast, idle", cast.castCalled, brain.CurrentIntention())
	}
}

func TestSummonAITryToCastApproachesBeforeCasting(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false)
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

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false)
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

	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = false, want queued cast intention")
	}
	strike.attackingNow = false
	cast.disabled = true
	brain.Think()
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle while cast controller is busy", got)
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

	if !brain.TryToCast(target, ref, false) {
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

	if brain.TryToCast(actor(200), skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = true with no CastController attached, want false")
	}
}

func TestSummonAITryToCastRejectsWhenCanAttemptFails(t *testing.T) {
	target := actor(200)
	cast := &recordingCast{canAttempt: false, canCast: true}
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = true when CanAttempt rejects the skill, want false")
	}
	if cast.castedTarget != nil {
		t.Fatalf("cast target = %v, want no cast dispatched", cast.castedTarget)
	}
}

// TestSummonAIFinishedCastingAppliesTheFollowItself pins that a cast ending
// with nothing to resume leaves the summon already following: the idle is
// decided and applied in one critical section, so a Betray TryToAttack from
// the caster's queue that lands once FinishedCasting returns keeps its attack
// instead of being overwritten by a later follow from the owner's queue.
func TestSummonAIFinishedCastingAppliesTheFollowItself(t *testing.T) {
	self := actor(100)
	owner := actor(1)
	move := &summonMove{}
	brain := NewSummon(self, move, &recordingAttack{canAttack: true})

	if !brain.FinishedCasting(owner) {
		t.Fatal("FinishedCasting() = false, want the summon sent idle")
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() after FinishedCasting = %v, want follow already applied", got)
	}
	if move.friendlyTarget != owner {
		t.Fatalf("friendly follow target = %v, want the owner", move.friendlyTarget)
	}

	if !brain.TryToAttack(owner) {
		t.Fatal("Betray TryToAttack(owner) = false, want accepted")
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after Betray = %v, want attack", got)
	}
}

// TestSummonAIFinishedCastingWithoutFollowStandsStill pins the follow-off
// idle: nothing to resume and nobody to follow stops the summon in place.
func TestSummonAIFinishedCastingWithoutFollowStandsStill(t *testing.T) {
	move := &summonMove{}
	brain := NewSummon(actor(100), move, &recordingAttack{})

	if !brain.FinishedCasting(nil) {
		t.Fatal("FinishedCasting(nil) = false, want the summon sent idle")
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
	if move.stopCount != 1 || move.friendlyTarget != nil {
		t.Fatalf("Stop calls = %d, follow target = %v; want 1 stop and no follow", move.stopCount, move.friendlyTarget)
	}
}

// castingAfterAttack returns a summon AI that was attacking target when its
// owner commanded a cast on it, so the cast replaced the attack.
func castingAfterAttack(t *testing.T) (*Summon, *fakeActor, *recordingAttack, *recordingCast) {
	t.Helper()
	self, target := actor(100), actor(200)
	self.known[target.ObjectID()] = true
	strike := &recordingAttack{canAttack: true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(self, &summonMove{}, strike)
	brain.SetCastController(cast)
	if !brain.TryToAttack(target) || strike.doAttackCalls != 1 {
		t.Fatalf("TryToAttack() did not swing (DoAttack calls %d)", strike.doAttackCalls)
	}
	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 1}, false) || cast.castCalls != 1 {
		t.Fatal("TryToCast() did not cast")
	}
	cast.casting = true
	return brain, target, strike, cast
}

// TestSummonAICastStoppedResumesTheReplacedAttack pins a stopped cast's AI
// step: with nothing queued it resumes the attack the cast replaced and
// reports nothing idled.
func TestSummonAICastStoppedResumesTheReplacedAttack(t *testing.T) {
	brain, target, strike, cast := castingAfterAttack(t)
	cast.casting = false

	idled, handled := brain.CastStopped(actor(1))
	if !handled || idled {
		t.Fatalf("CastStopped() = (idled %v, handled %v), want (false, true)", idled, handled)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want the attack resumed", got)
	}
	if strike.doAttackCalls != 2 || strike.target != target {
		t.Fatalf("DoAttack calls = %d on %v, want a second swing on the target", strike.doAttackCalls, strike.target)
	}
}

// TestSummonAICastStoppedDropsTheQueuedCast pins that a cast queued behind
// a stopped cast is dropped, not started: the summon goes idle, following
// its owner, rather than resuming the attack the first cast replaced or
// firing the queued skill through the interrupt. Without an owner to follow
// it stands still.
func TestSummonAICastStoppedDropsTheQueuedCast(t *testing.T) {
	for _, tc := range []struct {
		name   string
		follow attackable.Combatant
		want   Intention
	}{
		{name: "follows owner", follow: actor(1), want: IntentionFollow},
		{name: "no owner", follow: nil, want: IntentionIdle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			brain, target, strike, cast := castingAfterAttack(t)
			if brain.TryToCast(target, skill.Ref{ID: 4140, Level: 1}, false); cast.castCalls != 1 {
				t.Fatalf("TryToCast() while casting started a cast (calls %d), want it queued", cast.castCalls)
			}
			cast.casting = false

			idled, handled := brain.CastStopped(tc.follow)
			if !handled || !idled {
				t.Fatalf("CastStopped() = (idled %v, handled %v), want (true, true)", idled, handled)
			}
			if cast.castCalls != 1 {
				t.Fatalf("Cast calls = %d, want the queued cast dropped", cast.castCalls)
			}
			if strike.doAttackCalls != 1 {
				t.Fatalf("DoAttack calls = %d, want no resumed swing", strike.doAttackCalls)
			}
			if got := brain.CurrentIntention(); got != tc.want {
				t.Fatalf("CurrentIntention() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSummonAIFinishedCastingStartsTheQueuedCast pins the other side: a
// cast that completes starts the cast queued behind it.
func TestSummonAIFinishedCastingStartsTheQueuedCast(t *testing.T) {
	brain, target, _, cast := castingAfterAttack(t)
	if brain.TryToCast(target, skill.Ref{ID: 4140, Level: 1}, false); cast.castCalls != 1 {
		t.Fatalf("TryToCast() while casting started a cast (calls %d), want it queued", cast.castCalls)
	}
	cast.casting = false

	if idled := brain.FinishedCasting(actor(1)); idled {
		t.Fatal("FinishedCasting() idled, want the queued cast started")
	}
	if cast.castCalls != 2 || cast.castedRef.ID != 4140 {
		t.Fatalf("Cast calls = %d (last %v), want the queued skill 4140 cast", cast.castCalls, cast.castedRef)
	}
}

// TestSummonAICastStoppedWithoutAttackFollowsOwner pins a stopped cast with
// no attack to resume: the summon goes back to following its owner.
func TestSummonAICastStoppedWithoutAttackFollowsOwner(t *testing.T) {
	self, target, owner := actor(100), actor(200), actor(1)
	self.known[target.ObjectID()] = true
	move := &summonMove{}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(self, move, &recordingAttack{})
	brain.SetCastController(cast)
	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 1}, false) {
		t.Fatal("TryToCast() did not cast")
	}

	idled, handled := brain.CastStopped(owner)
	if !handled || !idled {
		t.Fatalf("CastStopped() = (idled %v, handled %v), want (true, true)", idled, handled)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow || move.friendlyTarget != owner {
		t.Fatalf("CurrentIntention() = %v following %v, want follow on the owner", got, move.friendlyTarget)
	}
}

// TestSummonAIAbortAllLeavesTheStoppedCastAlone pins that a cast AbortAll
// stops moves nothing on: the cast end it reports is left unhandled, so no
// attack resumes before AbortAll's caller settles the summon, while a cast
// stopped outside AbortAll afterwards is handled again.
func TestSummonAIAbortAllLeavesTheStoppedCastAlone(t *testing.T) {
	brain, _, strike, cast := castingAfterAttack(t)
	var handled, called bool
	cast.onStop = func() {
		called = true
		_, handled = brain.CastStopped(actor(1))
	}

	brain.AbortAll()
	if !called || handled {
		t.Fatalf("cast end during AbortAll: reported %v, handled %v; want reported and unhandled", called, handled)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("DoAttack calls = %d after AbortAll, want no resumed swing", strike.doAttackCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v after AbortAll, want the cast's idle left as is", got)
	}

	cast.onStop = nil
	if _, handled := brain.CastStopped(actor(1)); !handled {
		t.Fatal("CastStopped() after AbortAll returned unhandled, want the abort guard released")
	}
}
