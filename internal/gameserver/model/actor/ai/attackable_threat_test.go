package ai

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

func TestAttackableAITickDecaysThreatEveryThirdTick(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.AddDamageHate(target, 0, 20)

	ai.Tick()
	ai.Tick()
	if got := ai.Threats().Hate(target); got != 20 {
		t.Fatalf("hate after two ticks = %v, want 20", got)
	}

	ai.Tick()
	if got, want := ai.Threats().Hate(target), 13.4; math.Abs(got-want) > 0.000001 {
		t.Fatalf("hate after third tick = %v, want %v", got, want)
	}
}

func TestAttackableAITickRefreshesStaleThreatAndHate(t *testing.T) {
	owner := actor(1)
	lost := actor(2)
	dead := actor(3)
	dead.alikeDead = true
	kept := actor(4)
	owner.known = map[int32]bool{lost.ObjectID(): false, dead.ObjectID(): true, kept.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.AddDamageHate(lost, 7, 70)
	ai.AddDamageHate(dead, 8, 80)
	ai.AddDamageHate(kept, 9, 90)
	ai.AddHate(lost, 700)
	ai.AddHate(dead, 800)
	ai.AddHate(kept, 900)

	ai.Tick()
	ai.Tick()
	ai.Tick()

	if _, ok := ai.Threats().Get(lost); ok {
		t.Fatal("lost threat entry still present after refresh")
	}
	gotDead, ok := ai.Threats().Get(dead)
	if !ok {
		t.Fatal("dead threat entry was dropped, want damage preserved")
	}
	if gotDead.Hate != -6.6 || gotDead.Damage != 8 {
		t.Fatalf("dead threat entry = %+v, want hate refreshed then decayed and damage preserved", gotDead)
	}
	if got := ai.Threats().Hate(kept); math.Abs(got-83.4) > 0.000001 {
		t.Fatalf("kept threat hate = %v, want decay after refresh", got)
	}
	if got := ai.Hates().Hate(lost); got != 0 {
		t.Fatalf("lost hate entry = %v, want removed", got)
	}
	if got := ai.Hates().Hate(dead); got != 0 {
		t.Fatalf("dead hate entry = %v, want removed", got)
	}
	if got := ai.Hates().Hate(kept); got != 900 {
		t.Fatalf("kept hate entry = %v, want unchanged", got)
	}
}

func TestAttackableAIAddDefaultHateUsesTerritoryOpeningValue(t *testing.T) {
	owner := actor(1)
	first := actor(2)
	second := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDefaultHate(first)
	ai.AddDefaultHate(second)

	if got := ai.Hates().Hate(first); got != 300 {
		t.Fatalf("first default hate = %v, want 300", got)
	}
	if got := ai.Hates().Hate(second); got != 100 {
		t.Fatalf("second default hate = %v, want 100", got)
	}
}

func TestAttackableAIAddDefaultHateOutsideTerritoryUsesBaseValue(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDefaultHate(target)

	if got := ai.Hates().Hate(target); got != 100 {
		t.Fatalf("default hate outside territory = %v, want 100", got)
	}
}

func TestAttackableAISetBackToPeaceClearsCombatState(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.inTerritory = false
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{canAttack: true})

	ai.AddDamageHate(target, 5, 20)
	ai.AddHate(target, 30)
	ai.Think()

	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before reset = %v, want %v", got, IntentionAttack)
	}

	ai.SetBackToPeace()

	if !ai.Threats().IsEmpty() {
		t.Fatal("threat table not cleared")
	}
	if !ai.Hates().IsEmpty() {
		t.Fatal("hate table not cleared")
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0", got)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after reset = %v, want %v", got, IntentionWander)
	}
	if _, _, ok := ai.NextIntention(); ok {
		t.Fatal("NextIntention() ok = true after reset, want false")
	}
	if move.stopCount != 2 {
		t.Fatalf("stop count = %d, want 2", move.stopCount)
	}
}

func TestAttackableAIRandomizeHateDisplacesTargetAndRebuildsDesires(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(low, 0, 10)
	ai.AddDamageHate(high, 0, 25)

	always := func(attackable.Combatant) bool { return true }
	first := func(int) int { return 0 }
	if ok := ai.RandomizeHate(always, first); !ok {
		t.Fatal("RandomizeHate: ok = false, want true")
	}

	if got := ai.Threats().Hate(low); got != 225 {
		t.Fatalf("displaced attacker hate = %v, want 225", got)
	}
	if got := ai.Threats().Hate(high); got != 25 {
		t.Fatalf("mostHated hate = %v, want unchanged 25", got)
	}

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after RandomizeHate")
	}
	if desire.FinalTarget != low || desire.Weight != 225 {
		t.Fatalf("top desire = (%v, %v), want (low, 225)", desire.FinalTarget, desire.Weight)
	}
	if got := ai.Desires().Len(); got != 2 {
		t.Fatalf("desires len = %d, want 2 (requeued from threat table)", got)
	}
}

func TestAttackableAIRandomizeHateNoopWithSingleAttacker(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(target, 0, 10)

	always := func(attackable.Combatant) bool { return true }
	first := func(int) int { return 0 }
	if ok := ai.RandomizeHate(always, first); ok {
		t.Fatal("RandomizeHate: ok = true, want false with a single attacker")
	}
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (untouched)", got)
	}
}

func TestAttackableAIReconsiderTargetSwapsAndDropsPreviousDesire(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(low, 0, 10)
	ai.AddDamageHate(high, 0, 25)

	always := func(attackable.Combatant) bool { return true }
	chosen, ok := ai.ReconsiderTarget(always, always)
	if !ok {
		t.Fatal("ReconsiderTarget: ok = false, want true")
	}
	if chosen != low {
		t.Fatalf("chosen = %v, want low", chosen)
	}

	if got := ai.Threats().Hate(low); got != 10 {
		t.Fatalf("chosen hate = %v, want unchanged 10", got)
	}
	if got := ai.Threats().Hate(high); got != 0 {
		t.Fatalf("previous mostHated hate = %v, want zeroed 0", got)
	}
	if _, ok := ai.Desires().Peek(); !ok {
		t.Fatal("Desires().Peek() ok = false, want the new target's desire queued")
	}
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (previous mostHated's desire dropped)", got)
	}
	desire, _ := ai.Desires().Peek()
	if desire.FinalTarget != low {
		t.Fatalf("queued desire target = %v, want low", desire.FinalTarget)
	}
	if desire.Weight != 10 {
		t.Fatalf("queued desire weight = %v, want 10 (unchanged from AddDamageHate; "+
			"AggroList.java:169-177 never re-queues the chosen's desire, so "+
			"ReconsiderTarget must not double it via AddOrUpdate's accumulation)", desire.Weight)
	}
}

func TestAttackableAIReconsiderTargetNoopWithSingleAttacker(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(target, 0, 10)

	always := func(attackable.Combatant) bool { return true }
	if _, ok := ai.ReconsiderTarget(always, always); ok {
		t.Fatal("ReconsiderTarget: ok = true, want false with a single attacker")
	}
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (untouched)", got)
	}
}
