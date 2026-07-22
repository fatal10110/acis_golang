package ai

import (
	"sync"
	"testing"
)

func TestDesireQueueAddOrUpdateMergesEqualDesireInPlace(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)

	first := &Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10}
	q.AddOrUpdate(first)

	second := &Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 5}
	q.AddOrUpdate(second)

	if got := q.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1 (equal Desires must merge, not accumulate)", got)
	}

	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got != first {
		t.Fatalf("Peek() returned %p, want the original queued Desire %p (weight must merge in place, not reallocate)", got, first)
	}
	if got.Weight != 15 {
		t.Fatalf("Weight = %v, want 15 (10 + 5 merged)", got.Weight)
	}
}

func TestDesireQueueAddOrUpdateKeepsDistinctDesiresSeparate(t *testing.T) {
	q := NewDesireQueue()
	low := actor(1)
	high := actor(2)

	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: low, Weight: 10})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: high, Weight: 25})

	if got := q.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
}

func TestDesireQueuePeekReturnsHighestWeight(t *testing.T) {
	q := NewDesireQueue()
	low := actor(1)
	mid := actor(2)
	high := actor(3)

	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: low, Weight: 10})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: high, Weight: 25})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: mid, Weight: 15})

	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got.FinalTarget != high {
		t.Fatalf("Peek() target = %v, want highest-weight target", got.FinalTarget)
	}
}

func TestDesireQueuePeekEmpty(t *testing.T) {
	q := NewDesireQueue()

	if _, ok := q.Peek(); ok {
		t.Fatal("Peek() ok = true on empty queue, want false")
	}
}

func TestDesireQueueRespectsCapacity(t *testing.T) {
	q := NewDesireQueue()

	for i := int32(0); i < maxDesires+10; i++ {
		q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: actor(i), Weight: float64(i)})
	}

	if got := q.Len(); got != maxDesires {
		t.Fatalf("Len() = %d, want %d (capped)", got, maxDesires)
	}

	// A merge into an already-queued Desire must still succeed once the
	// queue is at capacity: capacity only blocks brand-new entries.
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: actor(0), Weight: 100})
	if got := q.Len(); got != maxDesires {
		t.Fatalf("Len() after merge at capacity = %d, want %d", got, maxDesires)
	}
	got, _ := q.Peek()
	if got.FinalTarget.ObjectID() != 0 || got.Weight != 100 {
		t.Fatalf("Peek() = (%v, %v), want (actor 0, weight 100)", got.FinalTarget, got.Weight)
	}
}

func TestDesireQueueDecreaseWeightByTypeRemovesBelowZero(t *testing.T) {
	q := NewDesireQueue()
	survivor := actor(1)
	victim := actor(2)

	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: survivor, Weight: 10})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: victim, Weight: 3})
	q.AddOrUpdate(&Desire{Kind: IntentionWander, Weight: 100})

	q.DecreaseWeightByType(IntentionAttack, 6.6)

	if got := q.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 (one ATTACK Desire dropped below zero, WANDER untouched)", got)
	}

	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got.Kind != IntentionWander {
		t.Fatalf("Peek() kind = %v, want wander (highest remaining weight)", got.Kind)
	}
}

func TestDesireQueueRemoveByKindAndTarget(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)
	other := actor(2)
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 100})
	q.AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Weight: 200})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: other, Weight: 50})

	q.Remove(IntentionAttack, target)

	if got := q.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got.Kind != IntentionCast || got.FinalTarget != target {
		t.Fatalf("Peek() = (%v, %v), want cast desire for removed attack target still present", got.Kind, got.FinalTarget)
	}
}

func TestDesireQueueRemoveFinalTarget(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)
	other := actor(2)
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 100})
	q.AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Weight: 200})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: other, Weight: 50})

	q.RemoveFinalTarget(target)

	if got := q.Len(); got != 1 {
		t.Fatalf("Len() = %d, want only the other target left", got)
	}
	got, ok := q.Peek()
	if !ok || got.FinalTarget != other {
		t.Fatalf("Peek() = (%v, %v), want other target", got, ok)
	}
}

func TestDesireQueueRemoveKind(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 100})
	q.AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Weight: 200})

	q.RemoveKind(IntentionAttack)

	if got := q.Len(); got != 1 {
		t.Fatalf("Len() = %d, want only cast desire left", got)
	}
	got, ok := q.Peek()
	if !ok || got.Kind != IntentionCast {
		t.Fatalf("Peek() = (%v, %v), want cast desire", got, ok)
	}
}

func TestDesireQueueConcurrentAccess(t *testing.T) {
	q := NewDesireQueue()

	var wg sync.WaitGroup
	for i := int32(0); i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			target := actor(id % 10)
			q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10})
			q.Peek()
			q.Len()
			q.DecreaseWeightByType(IntentionAttack, 1)
		}(i)
	}
	wg.Wait()
}
