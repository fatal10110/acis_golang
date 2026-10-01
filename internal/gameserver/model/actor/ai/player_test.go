package ai

import (
	"testing"

	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
)

func TestPlayerAttackRefusedTargetKeepsCurrentTarget(t *testing.T) {
	pk := gatePlayerFake(1, 30, 500)
	prev := gatePlayerFake(3, 30, 0)
	blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pk, &recordingMove{}, strike)

	if !brain.Start(prev, false) {
		t.Fatal("Start(prev) = false, want accepted")
	}
	strike.attackingNow = false
	if brain.Start(blessed, false) {
		t.Fatal("Start(blessed) = true, want refused")
	}
	if pk.refusals != 1 {
		t.Fatalf("refusals = %d, want 1", pk.refusals)
	}
	if got := brain.Target(); got != prev {
		t.Fatalf("Target() after refusal = %v, want the previous target", got)
	}
	if strike.doAttackCalls != 1 || strike.target != prev {
		t.Fatalf("swings = %d at %v, want only the one at the previous target", strike.doAttackCalls, strike.target)
	}

	if !brain.RefuseTarget(blessed) || pk.refusals != 2 {
		t.Fatalf("RefuseTarget(blessed) refusals = %d, want refused and reported", pk.refusals)
	}
	if brain.RefuseTarget(prev) || pk.refusals != 2 {
		t.Fatalf("RefuseTarget(prev) refusals = %d, want accepted silently", pk.refusals)
	}
	if got := brain.Target(); got != prev {
		t.Fatalf("Target() after RefuseTarget = %v, want the previous target", got)
	}
}

func TestPlayerAttackSkipsGateWhileDeniedOrBusy(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(pk *gateFake, strike *recordingAttack)
	}{
		{"denied", func(pk *gateFake, _ *recordingAttack) { pk.denied = true }},
		{"casting", func(pk *gateFake, _ *recordingAttack) { pk.casting = true }},
		{"attacking", func(_ *gateFake, strike *recordingAttack) { strike.attackingNow = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pk := gatePlayerFake(1, 30, 500)
			blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
			strike := &recordingAttack{canAttack: true}
			tc.set(pk, strike)
			brain := NewPlayerAttack(pk, &recordingMove{}, strike)

			if brain.RefuseTarget(blessed) {
				t.Fatal("RefuseTarget() = true, want the gate skipped")
			}
			brain.Start(blessed, false)
			if pk.refusals != 0 {
				t.Fatalf("refusals = %d, want 0 while %s", pk.refusals, tc.name)
			}
			if strike.doAttackCalls != 0 {
				t.Fatalf("swings = %d, want none while %s", strike.doAttackCalls, tc.name)
			}
		})
	}
}

// A swing that ends with nothing queued swings again only at a target the
// player can keep attacking; any other goes idle silently.
func TestPlayerAttackFinishedAttackKeepsOnlyKeepableTargets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		karma int
		swing bool
	}{
		{"unflagged player", 0, false},
		{"karma player", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc := gatePlayerFake(1, 40, 0)
			target := gatePlayerFake(2, 40, tc.karma)
			strike := &recordingAttack{canAttack: true}
			move := &recordingMove{}
			brain := NewPlayerAttack(pc, move, strike)
			if !brain.Start(target, false) {
				t.Fatal("Start() = false, want the first swing")
			}
			strike.attackingNow = false

			if brain.FinishedAttack() {
				t.Fatal("FinishedAttack() = true, want no ActionFailed")
			}
			wantSwings := 1
			if tc.swing {
				wantSwings = 2
			}
			if strike.doAttackCalls != wantSwings {
				t.Fatalf("swings = %d, want %d", strike.doAttackCalls, wantSwings)
			}
			if kept := brain.Target() != nil; kept != tc.swing {
				t.Fatalf("attack intention kept = %v, want %v", kept, tc.swing)
			}
		})
	}
}

// An attack requested again mid-swing is the next intention: it runs when
// the swing ends, keepable target or not.
func TestPlayerAttackFinishedAttackRunsTheQueuedAttack(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 0)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	if !brain.Start(target, false) {
		t.Fatal("Start() = false, want the first swing")
	}
	brain.Start(target, false)
	strike.attackingNow = false

	brain.FinishedAttack()
	if strike.doAttackCalls != 2 {
		t.Fatalf("swings = %d, want the queued attack to swing again", strike.doAttackCalls)
	}
}

// A dead unflagged player goes idle silently; a dead karma player is thought
// once more and lost, answered ActionFailed.
func TestPlayerAttackFinishedAttackOnDeadTarget(t *testing.T) {
	for _, tc := range []struct {
		name         string
		karma        int
		actionFailed bool
	}{
		{"unflagged player", 0, false},
		{"karma player", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc := gatePlayerFake(1, 40, 0)
			target := &deadGateFake{gateFake: gatePlayerFake(2, 40, tc.karma)}
			strike := &recordingAttack{canAttack: true}
			brain := NewPlayerAttack(pc, &recordingMove{}, strike)
			if !brain.Start(target, false) {
				t.Fatal("Start() = false, want the first swing")
			}
			strike.attackingNow = false
			target.dead = true

			if got := brain.FinishedAttack(); got != tc.actionFailed {
				t.Fatalf("FinishedAttack() = %v, want %v", got, tc.actionFailed)
			}
			if brain.Target() != nil {
				t.Fatal("attack intention kept on a dead target")
			}
		})
	}
}

type deadGateFake struct {
	*gateFake
	dead bool
}

func (d *deadGateFake) AlikeDead() bool { return d.dead }
func (d *deadGateFake) Dead() bool      { return d.dead }

// A shift-held attack never walks: on a target out of reach it goes idle,
// answered ActionFailed, with no follow and no swing. Within reach it swings
// as an unshifted attack does.
func TestPlayerAttackShiftHeldNeverWalks(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 500)
	strike := &recordingAttack{canAttack: true}
	move := &recordingMove{outOfReach: true, followStarted: true}
	brain := NewPlayerAttack(pc, move, strike)

	if brain.Start(target, true) {
		t.Fatal("Start(shift) on a target out of reach = true, want ActionFailed")
	}
	if move.followCalls != 0 || move.holdCalls != 1 {
		t.Fatalf("follow calls = %d, hold calls = %d, want 0 and 1", move.followCalls, move.holdCalls)
	}
	if strike.doAttackCalls != 0 {
		t.Fatalf("swings = %d, want none", strike.doAttackCalls)
	}
	if brain.Target() != nil {
		t.Fatal("shift-held attack out of reach kept its intention, want idle")
	}
	if move.stopCount == 0 {
		t.Fatal("idle did not stop movement")
	}

	move.outOfReach = false
	if !brain.Start(target, true) {
		t.Fatal("Start(shift) within reach = false, want the swing")
	}
	if strike.doAttackCalls != 1 || move.followCalls != 0 {
		t.Fatalf("swings = %d, follow calls = %d, want 1 and 0", strike.doAttackCalls, move.followCalls)
	}

	// The target steps out of reach before the swing ends: the re-think
	// goes idle instead of chasing it.
	strike.attackingNow = false
	move.outOfReach = true
	if !brain.FinishedAttack() {
		t.Fatal("FinishedAttack() on a shift-held attack out of reach = false, want ActionFailed")
	}
	if move.followCalls != 0 || brain.Target() != nil {
		t.Fatalf("follow calls = %d, intention kept = %v, want no follow and idle", move.followCalls, brain.Target() != nil)
	}
}

// The attack a nextActionAttack cast hands on to holds the cast's shift.
func TestPlayerAttackAfterShiftCastNeverWalks(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 500)
	strike := &recordingAttack{canAttack: true}
	move := &recordingMove{outOfReach: true, followStarted: true}
	brain := NewPlayerAttack(pc, move, strike)

	if !brain.AttackAfterCast(target, true) {
		t.Fatal("AttackAfterCast(shift) out of reach = no ActionFailed, want ActionFailed")
	}
	if move.followCalls != 0 || strike.doAttackCalls != 0 || brain.Target() != nil {
		t.Fatalf("follow calls = %d, swings = %d, intention kept = %v, want none and idle", move.followCalls, strike.doAttackCalls, brain.Target() != nil)
	}

	if brain.AttackAfterCast(target, false) {
		t.Fatal("AttackAfterCast(no shift) out of reach = ActionFailed, want the walk")
	}
	if move.followCalls != 1 {
		t.Fatalf("follow calls = %d, want the walk toward the target", move.followCalls)
	}
}
