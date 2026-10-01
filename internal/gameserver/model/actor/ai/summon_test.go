package ai

import (
	"bytes"
	"testing"

	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// ---- from summon_test.go ----
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

// TestSummonThinkStopsMovementAndAttacksOnTheSameTick pins
// thinkAttackLocked's two-call shape (mirroring Attackable.thinkAttack): an
// accepted swing cancels the walk and starts the attack in the same tick,
// and logs nothing for either.
func TestSummonThinkStopsMovementAndAttacksOnTheSameTick(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)
	var buf bytes.Buffer
	brain.SetLogger(zerolog.New(&buf))

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}

	if move.stopCount != 1 {
		t.Fatalf("move.Stop calls = %d, want 1", move.stopCount)
	}
	if strike.doAttackCalls != 1 || strike.target != target {
		t.Fatalf("DoAttack calls = (%d, %v), want (1, target)", strike.doAttackCalls, strike.target)
	}
	if logged := buf.String(); logged != "" {
		t.Fatalf("logged = %q, want nothing logged on the accepted path", logged)
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

// TestSummonAIRecheckOffensiveFollowReissuesOnMovingTarget is the coverage
// for #1960: CreatureMove.java's 500 ms ATTACK_FOLLOW_INTERVAL
// (CreatureMove.java:41,556-561) re-evaluates an in-flight offensive follow
// on its own schedule, independent of the shared 1 s AI think tick. Each
// recheckOffensiveFollow call simulates one of those 500 ms ticks; a
// moving/out-of-range target keeps reissuing the follow request without
// waiting for Think.
func TestSummonAIRecheckOffensiveFollowReissuesOnMovingTarget(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	brain := NewSummon(owner, move, &recordingAttack{canAttack: true})

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if move.followCalls != 1 {
		t.Fatalf("followCalls after TryToAttack = %d, want 1", move.followCalls)
	}

	brain.recheckOffensiveFollow()
	brain.recheckOffensiveFollow()

	if move.followCalls != 3 {
		t.Fatalf("followCalls after two 500 ms rechecks = %d, want 3 (1 initial + 2 rechecks)", move.followCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack still in flight", got)
	}
}

// TestSummonAIRecheckOffensiveFollowDoesNotReattackInRange is the
// regression test for the review finding that recheckOffensiveFollow
// called the full thinkAttackLocked, which falls through to
// attack.DoAttack whenever the summon is already in range (following ==
// false) and not busy. Java's ATTACK_FOLLOW_INTERVAL task
// (offensiveFollowTask, CreatureMove.java:563-584) only manages movement
// and never initiates an attack, so a summon whose weapon clears its
// cooldown inside 500 ms must not get a second DoAttack from this ticker.
func TestSummonAIRecheckOffensiveFollowDoesNotReattackInRange(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{} // followStarted defaults to false: already in range.
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("doAttackCalls after TryToAttack = %d, want 1", strike.doAttackCalls)
	}

	brain.recheckOffensiveFollow()

	if strike.doAttackCalls != 1 {
		t.Fatalf("doAttackCalls after 500 ms recheck = %d, want 1 (recheck must not re-attack)", strike.doAttackCalls)
	}
}

// TestSummonAIRecheckOffensiveFollowDoesNotRecastInRange is the cast-path
// counterpart of TestSummonAIRecheckOffensiveFollowDoesNotReattackInRange:
// the 500 ms recheck must not fall through to cast.Cast either.
func TestSummonAIRecheckOffensiveFollowDoesNotRecastInRange(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	// Starts out of range (chasing), so TryToCast queues the approach
	// instead of casting immediately, matching thinkCastLocked's
	// following==true early return and leaving the cast intention active.
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)
	ref := skill.Ref{ID: 4139, Level: 8}

	if !brain.TryToCast(target, ref, false) {
		t.Fatal("TryToCast() = false, want accepted cast approach")
	}
	if cast.castCalls != 0 {
		t.Fatalf("castCalls after TryToCast approach = %d, want 0 (still chasing)", cast.castCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() after TryToCast approach = %v, want cast still in flight", got)
	}

	// Target now sits in range: the 500 ms recheck (recheckOffensiveFollow)
	// must only re-evaluate the follow, not fall through to cast.Cast —
	// that execution belongs exclusively to the shared 1 s Think cadence.
	move.followStarted = false
	brain.recheckOffensiveFollow()

	if cast.castCalls != 0 {
		t.Fatalf("castCalls after 500 ms recheck = %d, want 0 (recheck must not cast)", cast.castCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() after recheck = %v, want cast still pending for Think", got)
	}
}

// TestSummonAIRecheckOffensiveFollowIgnoresFriendlyFollow proves the 500 ms
// offensive-follow recheck is a no-op outside an attack/cast intention, so
// friendly (owner) follow stays on the shared 1 s AI think cadence.
func TestSummonAIRecheckOffensiveFollowIgnoresFriendlyFollow(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	brain := NewSummon(owner, move, &recordingAttack{})

	if !brain.TryToFollow(target) {
		t.Fatal("TryToFollow() = false, want accepted follow")
	}

	brain.recheckOffensiveFollow()

	if move.followCalls != 0 {
		t.Fatalf("followCalls after recheck during friendly follow = %d, want 0", move.followCalls)
	}
}

// TestSummonAITargetLostDuringOffensiveFollowRecheckCancelsStaleMove is the
// known-list-loss coverage for #1960: SummonMove.java:48-53's follow-task
// branch forces the idle path's move.stop() when a followed target drops
// out of the known list mid-chase, so a stale movement leg can't keep
// running toward a target the summon no longer knows about.
func TestSummonAITargetLostDuringOffensiveFollowRecheckCancelsStaleMove(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	brain := NewSummon(owner, move, &recordingAttack{canAttack: true})

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if move.stopCount != 0 {
		t.Fatalf("stopCount after TryToAttack = %d, want 0", move.stopCount)
	}

	owner.known[target.id] = false
	brain.recheckOffensiveFollow()

	if move.stopCount != 1 {
		t.Fatalf("stopCount after target lost = %d, want 1 (stale move canceled)", move.stopCount)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle after target lost", got)
	}
}

// ---- summon step-aside ----
// stepAsideMove records every walk a summon AI starts.
type stepAsideMove struct {
	summonMove
	walks []location.Location
}

func (m *stepAsideMove) MoveToLocation(dest location.Location) (bool, error) {
	m.walks = append(m.walks, dest)
	return true, nil
}

// TestSummonAIStepAsideOnlyFromIdleOrFollow pins SummonMove.avoidAttack's
// intention gate: an idle or following summon walks to the spot and takes a
// MOVE_TO intention, while one attacking, casting or unable to act refuses
// and keeps its intention.
func TestSummonAIStepAsideOnlyFromIdleOrFollow(t *testing.T) {
	dest := location.Location{X: 1070, Y: 1000, Z: 0}
	ref := skill.Ref{ID: 4139, Level: 8}

	tests := []struct {
		name  string
		setup func(t *testing.T, brain *Summon, owner *fakeActor, strike *recordingAttack, cast *recordingCast)
		want  Intention
		walks bool
	}{
		{name: "idle", want: IntentionMoveTo, walks: true},
		{
			name: "follow",
			setup: func(t *testing.T, brain *Summon, _ *fakeActor, _ *recordingAttack, _ *recordingCast) {
				if !brain.TryToFollow(actor(1)) {
					t.Fatal("TryToFollow() = false, want accepted follow")
				}
			},
			want:  IntentionMoveTo,
			walks: true,
		},
		{
			name: "attack",
			setup: func(t *testing.T, brain *Summon, _ *fakeActor, strike *recordingAttack, _ *recordingCast) {
				if !brain.TryToAttack(actor(200)) {
					t.Fatal("TryToAttack() = false, want accepted attack")
				}
				// Between swings: only the intention holds the summon back.
				strike.attackingNow = false
			},
			want: IntentionAttack,
		},
		{
			name: "cast",
			setup: func(t *testing.T, brain *Summon, _ *fakeActor, _ *recordingAttack, cast *recordingCast) {
				// The summon is still closing distance, so the cast intention
				// stays current with no cast in flight.
				brain.TryToCast(actor(200), ref, false)
				if cast.castCalled {
					t.Fatal("Cast() called while closing distance, want the approach only")
				}
			},
			want: IntentionCast,
		},
		{
			name: "idle but denied AI action",
			setup: func(_ *testing.T, _ *Summon, owner *fakeActor, _ *recordingAttack, _ *recordingCast) {
				owner.denyAction = true
			},
			want: IntentionIdle,
		},
		{
			name: "idle mid-swing",
			setup: func(_ *testing.T, _ *Summon, _ *fakeActor, strike *recordingAttack, _ *recordingCast) {
				strike.attackingNow = true
			},
			want: IntentionIdle,
		},
		{
			name: "idle mid-cast",
			setup: func(_ *testing.T, _ *Summon, _ *fakeActor, _ *recordingAttack, cast *recordingCast) {
				cast.casting = true
			},
			want: IntentionIdle,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner := actor(100)
			move := &stepAsideMove{summonMove: summonMove{recordingMove: recordingMove{followStarted: true}}}
			strike := &recordingAttack{canAttack: true}
			cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
			brain := NewSummon(owner, move, strike)
			brain.SetCastController(cast)
			if tc.setup != nil {
				tc.setup(t, brain, owner, strike, cast)
			}
			before := len(move.walks)

			got := brain.StepAside(dest)
			if got != tc.walks {
				t.Fatalf("StepAside() = %v, want %v", got, tc.walks)
			}
			if kind := brain.CurrentIntention(); kind != tc.want {
				t.Fatalf("CurrentIntention() = %v, want %v", kind, tc.want)
			}
			walks := move.walks[before:]
			if tc.walks {
				if len(walks) != 1 || walks[0] != dest {
					t.Fatalf("walks = %v, want [%v]", walks, dest)
				}
			} else if len(walks) != 0 {
				t.Fatalf("walks = %v, want none", walks)
			}
		})
	}
}

func TestSummonAIRefusedTargetKeepsCurrentIntention(t *testing.T) {
	owner := gatePlayerFake(1, 30, 500)
	pet := gateSummonFake(3, owner)
	blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(pet, &summonMove{}, strike)

	if !brain.TryToFollow(owner) {
		t.Fatal("TryToFollow(owner) = false, want accepted")
	}
	if brain.TryToAttack(blessed) {
		t.Fatal("TryToAttack(blessed) = true, want refused")
	}
	if pet.refusals != 1 {
		t.Fatalf("refusals = %d, want 1", pet.refusals)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() after refusal = %v, want follow", got)
	}
	if _, _, queued := brain.NextIntention(); queued {
		t.Fatal("refused attack was queued as the next intention")
	}
	if strike.doAttackCalls != 0 {
		t.Fatalf("swings = %d, want none", strike.doAttackCalls)
	}
}

func TestSummonAISkipsGateWhileDeniedOrBusy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		set    func(pet *gateFake, strike *recordingAttack)
		queued bool
	}{
		{"denied", func(pet *gateFake, _ *recordingAttack) { pet.denied = true }, false},
		{"attacking", func(_ *gateFake, strike *recordingAttack) { strike.attackingNow = true }, true},
		{"bow cooling down", func(_ *gateFake, strike *recordingAttack) { strike.bowCooling = true }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := gateSummonFake(3, gatePlayerFake(1, 30, 500))
			blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
			strike := &recordingAttack{canAttack: true}
			tc.set(pet, strike)
			brain := NewSummon(pet, &summonMove{}, strike)

			brain.TryToAttack(blessed)
			if pet.refusals != 0 {
				t.Fatalf("refusals = %d, want 0 while %s", pet.refusals, tc.name)
			}
			kind, next, ok := brain.NextIntention()
			if tc.queued && (!ok || kind != IntentionAttack || next != blessed) {
				t.Fatalf("NextIntention() = (%v,%v,%v), want the attack queued", kind, next, ok)
			}
			if strike.doAttackCalls != 0 {
				t.Fatalf("swings = %d, want none while %s", strike.doAttackCalls, tc.name)
			}
		})
	}
}

// A summon that cannot keep attacking its target goes idle once the swing
// ends: back to following its owner, or standing still with follow off.
func TestSummonAIFinishedAttackIdlesOnUnkeepableTarget(t *testing.T) {
	owner := gatePlayerFake(1, 40, 0)
	pet := gateSummonFake(3, owner)
	target := gatePlayerFake(2, 40, 0)
	strike := &recordingAttack{canAttack: true}
	move := &summonMove{}
	brain := NewSummon(pet, move, strike)
	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want the first swing")
	}
	strike.attackingNow = false

	if !brain.FinishedAttack(owner) {
		t.Fatal("FinishedAttack() = false, want the summon sent idle")
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want follow", got)
	}
	if move.friendlyTarget != owner {
		t.Fatalf("friendly follow target = %v, want the owner", move.friendlyTarget)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("swings = %d, want 1", strike.doAttackCalls)
	}

	still := NewSummon(gateSummonFake(4, owner), &summonMove{}, &recordingAttack{canAttack: true})
	still.TryToAttack(target)
	if !still.FinishedAttack(nil) || still.CurrentIntention() != IntentionIdle {
		t.Fatalf("follow-off FinishedAttack: intention = %v, want idle", still.CurrentIntention())
	}
}

// Against a keepable target, or with an intention queued, the swing's end
// carries on as before.
func TestSummonAIFinishedAttackKeepsKeepableTargets(t *testing.T) {
	owner := gatePlayerFake(1, 40, 0)
	pet := gateSummonFake(3, owner)
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(pet, &summonMove{}, strike)
	karma := gatePlayerFake(2, 40, 500)
	brain.TryToAttack(karma)
	strike.attackingNow = false

	if brain.FinishedAttack(owner) {
		t.Fatal("FinishedAttack() = true, want the attack kept")
	}
	if strike.doAttackCalls != 2 || brain.CurrentIntention() != IntentionAttack {
		t.Fatalf("swings = %d, intention = %v; want a second swing on the attack", strike.doAttackCalls, brain.CurrentIntention())
	}

	white := gatePlayerFake(5, 40, 0)
	strike.attackingNow = true
	brain.TryToAttack(white)
	strike.attackingNow = false
	if brain.FinishedAttack(owner) {
		t.Fatal("FinishedAttack() with a queued attack = true, want it run")
	}
	if strike.target != white {
		t.Fatalf("swing target = %v, want the queued attack's", strike.target)
	}
}
