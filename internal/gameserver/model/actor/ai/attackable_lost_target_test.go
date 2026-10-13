package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// committedAttack makes an attack on target current, past the first cycle
// and unlatched, with target hated in both tables.
func committedAttack(t *testing.T, owner, target *fakeActor, move *recordingMove, strike *recordingAttack) *Attackable {
	t.Helper()
	brain := NewAttackable(owner, move, strike)
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	attackUnlatched(t, brain, target)
	brain.AddHate(target, 50)
	return brain
}

// TestAttackStepOnDeadKnownTargetKeepsEverything pins the attack step's
// lost-target check (a target the actor still knows, though dead, is not
// lost): the continue pass leaves hate, desires and the intention as they
// are and reaches the follow and attack gates, which refuse the dead target.
func TestAttackStepOnDeadKnownTargetKeepsEverything(t *testing.T) {
	owner, target := actor(1), actor(2)
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	brain := committedAttack(t, owner, target, move, strike)
	threat := brain.Threats().Hate(target)
	follows, swings := move.followCalls, strike.doAttackCalls

	target.alikeDead = true
	strike.canAttack = false
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}

	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want the attack kept", got)
	}
	if got := brain.Threats().Hate(target); got != threat {
		t.Fatalf("threat hate = %v, want %v kept", got, threat)
	}
	if got := brain.Hates().Hate(target); got != 50 {
		t.Fatalf("skill hate = %v, want 50 kept", got)
	}
	if !brain.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("attack desire on the dead target dropped by the step")
	}
	if move.followCalls != follows+1 {
		t.Fatalf("follow gate calls = %d, want %d", move.followCalls, follows+1)
	}
	if strike.doAttackCalls != swings {
		t.Fatal("swung at the dead target")
	}
}

// TestAttackStepOnUnknownTargetChangesNothing pins the lost-target check's
// lack of side effects: an attack whose target the actor no longer knows
// keeps its intention, threat entry, skill hate and desire, and takes no
// further step.
func TestAttackStepOnUnknownTargetChangesNothing(t *testing.T) {
	owner, target := actor(1), actor(2)
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	brain := committedAttack(t, owner, target, move, strike)
	threat := brain.Threats().Hate(target)
	follows, swings := move.followCalls, strike.doAttackCalls

	owner.known[target.ObjectID()] = false
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}

	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want the attack kept", got)
	}
	if got := brain.Threats().Hate(target); got != threat || len(brain.Threats().Snapshot()) != 1 {
		t.Fatalf("threat hate = %v (%d entries), want %v in the one entry kept", got, len(brain.Threats().Snapshot()), threat)
	}
	if got := brain.Hates().Hate(target); got != 50 {
		t.Fatalf("skill hate = %v, want 50 kept", got)
	}
	if !brain.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("attack desire on the unknown target dropped by the step")
	}
	if move.followCalls != follows || strike.doAttackCalls != swings {
		t.Fatal("the step went past the lost-target check")
	}
}

// TestLostAttackTargetDoesNotReselect pins that desire selection takes one
// pick per pass: a latched attack whose target is lost ends the pass, and a
// heavier desire on another target waits for the next one.
func TestLostAttackTargetDoesNotReselect(t *testing.T) {
	owner, lost, other := actor(1), actor(2), actor(3)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	// Coming from idle, the first attack latches.
	brain.AddAttackDesire(lost, 10)
	if !brain.hasLatch() {
		t.Fatal("setup: first attack not latched")
	}
	owner.known[lost.ObjectID()] = false
	addAttackHate(brain, other, 0, 100)
	swings := strike.doAttackCalls

	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack || strike.doAttackCalls != swings {
		t.Fatalf("after the latched pass: intention %v, swings %d; want the lost attack, no swing", got, strike.doAttackCalls-swings)
	}

	if err := brain.RunAI(); err != nil {
		t.Fatalf("second RunAI() error: %v", err)
	}
	if strike.target != other {
		t.Fatalf("second pass attacked %v, want the other target", strike.target)
	}
}

// TestCastStepOnLostTargetChangesNothing pins the cast step's lost-target
// check: the intention, the skill hate and the cast desire stay as they are
// and no cast starts.
func TestCastStepOnLostTargetChangesNothing(t *testing.T) {
	owner, target := actor(1), actor(2)
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.SetCastController(cast)
	brain.AddHate(target, 50)
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	cast.disabled = true
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("setup: CurrentIntention() = %v, want cast", got)
	}

	cast.disabled = false
	owner.known[target.ObjectID()] = false
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if cast.castCalled {
		t.Fatal("cast started on a lost target")
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() = %v, want the cast kept", got)
	}
	if got := brain.Hates().Hate(target); got != 50 {
		t.Fatalf("skill hate = %v, want 50 kept", got)
	}
	if brain.Desires().Len() != 1 {
		t.Fatalf("desires = %d, want the cast desire kept", brain.Desires().Len())
	}
}

// TestSummonFriendCastOnTargetOutOfWorldIsLost pins that the SUMMON_FRIEND
// bypass covers only a target the actor does not know: one that left the
// world is lost.
func TestSummonFriendCastOnTargetOutOfWorldIsLost(t *testing.T) {
	owner, target := actor(1), actor(2)
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{disabled: true, canAttempt: true, canCast: true, skillType: "SUMMON_FRIEND"}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.SetCastController(cast)
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}

	cast.disabled = false
	owner.gone = map[int32]bool{target.ObjectID(): true}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if cast.castCalled {
		t.Fatal("SUMMON_FRIEND cast started on a target that left the world")
	}
}

// TestFirstAttackDesireOnUnknownAttackerRunsSelection pins the first-hate
// selection run for any attacker: with no most-hated creature, queueing an
// attack desire runs desire selection at once, and its prune drops the
// desire on an attacker the actor does not know; the threat entry stays.
func TestFirstAttackDesireOnUnknownAttackerRunsSelection(t *testing.T) {
	owner, attacker := actor(1), actor(2)
	owner.known[attacker.ObjectID()] = false
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	brain.AddAttackDesire(attacker, 100)

	if brain.Desires().Len() != 0 {
		t.Fatalf("desires = %d, want the unknown attacker's desire pruned at once", brain.Desires().Len())
	}
	if got := brain.Threats().Hate(attacker); got != 100 {
		t.Fatalf("threat hate = %v, want 100", got)
	}
}

// TestFirstCycleKeepsAttackWhoseDesireLeft pins that desire selection does
// not idle an intention whose desire left the queue: on the first periodic
// cycle, with promotion gated and no empty-queue idle yet, the attack stays
// current, and only the next cycle's empty-queue idle idles it.
func TestFirstCycleKeepsAttackWhoseDesireLeft(t *testing.T) {
	owner, target := actor(1), actor(2)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	brain.AddAttackDesire(target, 100)
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("setup: CurrentIntention() = %v, want attack", got)
	}
	brain.Desires().Clear()
	brain.latched = intention{}
	owner.hooks.points = nil

	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after the first cycle = %v, want the attack kept", got)
	}
	assertHookPoints(t, owner, "on the first cycle", HookSeeCreature)

	if err := brain.TickThink(); err != nil {
		t.Fatalf("second TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the second cycle = %v, want idle", got)
	}
}

// TestSelectionInsideHitAnimationKeepsAttackWhoseDesireLeft pins that a
// selection the hit animation blocks leaves the current attack as it is,
// though its desire left the queue and another desire waits.
func TestSelectionInsideHitAnimationKeepsAttackWhoseDesireLeft(t *testing.T) {
	owner, target, other := actor(1), actor(2), actor(3)
	strike := &hitAnimationAttack{recordingAttack: recordingAttack{canAttack: true}}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	attackUnlatched(t, brain, target)
	brain.Desires().Clear()
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionFollow, FinalTarget: other, Weight: 5})

	strike.inHitAnimation = true
	strike.attackingNow = true
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() inside the hit animation = %v, want the attack kept", got)
	}
}

// TestSelectionWithNothingPromotedTakesNoStep pins that desire selection
// steps the current intention only through a promotion: before the first
// periodic cycle, a cast whose desire its end dropped is neither idled nor
// cast again by the selection that end runs.
func TestSelectionWithNothingPromotedTakesNoStep(t *testing.T) {
	owner, target := actor(1), actor(2)
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.SetCastController(cast)
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if cast.castCalls != 1 {
		t.Fatalf("setup: casts = %d, want 1", cast.castCalls)
	}

	brain.ClearCurrentDesire()
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() after the cast end error: %v", err)
	}
	if cast.castCalls != 1 {
		t.Fatalf("casts after the cast end = %d, want no new cast", cast.castCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() = %v, want the cast kept", got)
	}
}
