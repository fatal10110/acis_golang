package ai

import (
	"testing"
	"time"
	"unsafe"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestAttackableLayoutSize pins the per-NPC AI loop size: one inline
// intention, the latch and queued next behind one pointer made on first use.
func TestAttackableLayoutSize(t *testing.T) {
	if got := unsafe.Sizeof(intention{}); got != 112 {
		t.Errorf("unsafe.Sizeof(intention{}) = %d, want 112", got)
	}
	if got := unsafe.Sizeof(Attackable{}); got != 416 {
		t.Errorf("unsafe.Sizeof(Attackable{}) = %d, want 416", got)
	}
}

// An idle actor never makes the pending pair: clearing an unset latch or
// next intention leaves it unmade, and a set reads back.
func TestPendingIntentionsMadeOnFirstSet(t *testing.T) {
	brain := NewAttackable(actor(1), &recordingMove{}, &recordingAttack{})
	brain.setLatched(intention{})
	brain.setNext(intention{})
	if brain.pending != nil || brain.hasLatch() {
		t.Fatal("clearing an unset latch or next intention made the pending pair")
	}
	if _, _, ok := brain.NextIntention(); ok {
		t.Fatal("an unset next intention reads as queued")
	}

	target := actor(2)
	brain.setNext(intention{kind: IntentionAttack, target: target})
	if kind, got, ok := brain.NextIntention(); !ok || kind != IntentionAttack || got != target {
		t.Fatalf("NextIntention() = %v, %v, %v, want attack on the target", kind, got, ok)
	}
	if brain.hasLatch() {
		t.Fatal("setting the next intention latched an attack")
	}
	brain.setLatched(intention{kind: IntentionAttack, target: target})
	if !brain.hasLatch() {
		t.Fatal("a set latch does not read back")
	}
	brain.setLatched(intention{})
	brain.setNext(intention{})
	if brain.hasLatch() {
		t.Fatal("a cleared latch still reads as set")
	}
	if _, _, ok := brain.NextIntention(); ok {
		t.Fatal("a cleared next intention still reads as queued")
	}
}

// A repeated cast desire folds its weight into the queued one in place: one
// entry, the first request's queue time, and no allocation per repeat.
func TestAddCastDesireRepeatFoldsInPlace(t *testing.T) {
	brain, _, _ := castDesireAI(t)
	target := actor(2)
	first := time.UnixMilli(1000)
	now := first
	brain.now = func() time.Time { return now }

	brain.AddCastDesire(target, desireRef, 10, false, true)
	now = first.Add(time.Second)
	brain.AddCastDesire(target, desireRef, 5, false, false)
	other := skill.Ref{ID: desireRef.ID + 1, Level: 1}
	brain.AddCastDesire(target, other, 1, false, true)

	queued := brain.Desires().Snapshot()
	if len(queued) != 2 {
		t.Fatalf("queued %d desires, want 2 (the folded cast and the other skill's)", len(queued))
	}
	d, _ := queuedCast(brain, target)
	if d.Skill != desireRef || d.Weight != 15 || !d.QueuedAt.Equal(first) || !d.MoveToTarget {
		t.Fatalf("folded cast = skill %v weight %v queued %v moving %v, want %v, 15, %v, true", d.Skill, d.Weight, d.QueuedAt, d.MoveToTarget, desireRef, first)
	}

	allocs := testing.AllocsPerRun(100, func() {
		brain.AddCastDesire(target, desireRef, 1, false, true)
	})
	if allocs != 0 {
		t.Fatalf("a repeated cast desire allocates %v times, want 0", allocs)
	}
	if d, _ := queuedCast(brain, target); d.Weight != 15+101 {
		t.Fatalf("weight after 101 repeats = %v, want %v", d.Weight, 15+101)
	}
}
