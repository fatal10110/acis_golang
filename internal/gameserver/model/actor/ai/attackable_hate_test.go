package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

func TestAttackableAISetBackToPeaceClearsCombatState(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.inTerritory = false
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 5, 20)
	ai.AddHate(target, 30)
	ai.RunAI()

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
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after reset = %v, want %v", got, IntentionIdle)
	}
	if _, _, ok := ai.NextIntention(); ok {
		t.Fatal("NextIntention() ok = true after reset, want false")
	}
	if move.stopCount != 2 {
		t.Fatalf("stop count = %d, want 2", move.stopCount)
	}
}

func TestAttackableAIReduceAllAggroHateReturnsToPeaceWhenExhausted(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 0, 5)
	ai.RunAI()
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before decay = %v, want %v", got, IntentionAttack)
	}

	ai.ReduceAllAggroHate(10)

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after hate exhausted = %v, want %v", got, IntentionIdle)
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 after peace", got)
	}
}

func TestAttackableAIStopAggroHateReturnsToPeaceWhenExhausted(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 0, 20)
	ai.RunAI()
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before stop = %v, want %v", got, IntentionAttack)
	}

	ai.StopAggroHate(target)

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after stop hate = %v, want %v", got, IntentionIdle)
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 after peace", got)
	}
}

func TestAttackableAIReduceAllAggroHateKeepsAttackWhenSkillHateRemains(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 0, 5)
	ai.AddHate(target, 50)
	ai.RunAI()
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before decay = %v, want %v", got, IntentionAttack)
	}

	ai.ReduceAllAggroHate(10)

	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() with skill hate remaining = %v, want %v", got, IntentionAttack)
	}
}

func TestAttackableAIRandomizeHateDisplacesTargetAndRebuildsDesires(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)

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

// TestAttackableAIAddDamageHateSetsMoveToTarget ports NpcAI.java:698-711:
// every addAttackDesire overload reached from the ordinary hate-list path
// (Npc.java:2036's getAI().addAttackDesire) defaults moveToTarget = true.
func TestAttackableAIAddDamageHateSetsMoveToTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, target, 0, 10)

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after AddDamageHate")
	}
	if !desire.MoveToTarget {
		t.Fatal("AddDamageHate queued desire MoveToTarget = false, want true")
	}
}

// TestAttackableAIAddAttackDesireHoldSetsMoveToTarget ports
// NpcAI.java:683-696: addAttackDesireHold queues MoveToTarget = false.
func TestAttackableAIAddAttackDesireHoldSetsMoveToTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(target, 0, 10)
	ai.AddAttackDesireHold(target, 10)

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after AddAttackDesireHold")
	}
	if desire.MoveToTarget {
		t.Fatal("AddAttackDesireHold queued desire MoveToTarget = true, want false")
	}
	if desire.FinalTarget != target || desire.Weight != 10 {
		t.Fatalf("hold desire = (%v, %v), want (target, 10)", desire.FinalTarget, desire.Weight)
	}
}

// TestAttackableAIRandomizeHateRequeuesWithMoveToTarget ports
// AggroList.randomizeAttack()'s post-swap requeue loop (AggroList.java:225-226),
// which resolves to the same moveToTarget = true overload.
func TestAttackableAIRandomizeHateRequeuesWithMoveToTarget(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)

	always := func(attackable.Combatant) bool { return true }
	first := func(int) int { return 0 }
	if ok := ai.RandomizeHate(always, first); !ok {
		t.Fatal("RandomizeHate: ok = false, want true")
	}

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after RandomizeHate")
	}
	if !desire.MoveToTarget {
		t.Fatal("RandomizeHate requeued desire MoveToTarget = false, want true")
	}
}

func TestAttackableAIRandomizeHateNoopWithSingleAttacker(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, target, 0, 10)

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

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)

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

	addAttackHate(ai, target, 0, 10)

	always := func(attackable.Combatant) bool { return true }
	if _, ok := ai.ReconsiderTarget(always, always); ok {
		t.Fatal("ReconsiderTarget: ok = true, want false with a single attacker")
	}
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (untouched)", got)
	}
}
