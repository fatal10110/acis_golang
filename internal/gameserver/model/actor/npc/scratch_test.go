package npc

import (
	"sync"
	"testing"
)

type scratchRef int32

func (r scratchRef) ObjectID() int32 { return int32(r) }

func TestNewScratchStartsWithWeightPointOne(t *testing.T) {
	s := NewScratch()
	for slot := range intSlotCount {
		want := int32(0)
		if slot == IntWeightPoint {
			want = 1
		}
		if got := s.Int(slot); got != want {
			t.Errorf("Int(%d) = %d, want %d", slot, got, want)
		}
	}
	for slot := range creatureSlotCount {
		if got := s.Creature(slot); got != nil {
			t.Errorf("Creature(%d) = %v, want none", slot, got)
		}
	}
	if got := s.ScriptValue(); got != 0 {
		t.Errorf("ScriptValue() = %d, want 0", got)
	}
}

// A respawn of the slot clears the script value and keeps every other
// slot, while other goroutines read and write the memory.
func TestScratchRespawnedKeepsSlotsButScriptValue(t *testing.T) {
	s := NewScratch()
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Go(func() {
			for i := range 200 {
				s.SetInt(IntAI0+IntSlot(w), int32(i))
				s.SetCreature(CreatureAI0+CreatureSlot(w), scratchRef(i))
				s.SetScriptValue(int32(i))
				s.CompareAndExchangeInt(IntAVQuest0, 0, 1)
				_ = s.Int(IntQuest0)
				_ = s.Creature(CreatureQuest0)
				s.Respawned()
			}
		})
	}
	wg.Wait()

	for slot := range intSlotCount {
		s.SetInt(slot, int32(slot)+100)
	}
	for slot := range creatureSlotCount {
		s.SetCreature(slot, scratchRef(slot+1))
	}
	s.SetScriptValue(7)

	s.Respawned()

	if got := s.ScriptValue(); got != 0 {
		t.Fatalf("ScriptValue() after Respawned = %d, want 0", got)
	}
	for slot := range intSlotCount {
		if got := s.Int(slot); got != int32(slot)+100 {
			t.Errorf("Int(%d) after Respawned = %d, want %d", slot, got, int32(slot)+100)
		}
	}
	for slot := range creatureSlotCount {
		if got := s.Creature(slot); got != scratchRef(slot+1) {
			t.Errorf("Creature(%d) after Respawned = %v, want %d", slot, got, slot+1)
		}
	}
}

func TestScratchCompareAndExchangeIntReturnsWitness(t *testing.T) {
	s := NewScratch()
	if got := s.CompareAndExchangeInt(IntAVQuest0, 0, 1); got != 0 {
		t.Fatalf("first exchange witness = %d, want 0", got)
	}
	if got := s.CompareAndExchangeInt(IntAVQuest0, 0, 2); got != 1 {
		t.Fatalf("second exchange witness = %d, want 1", got)
	}
	if got := s.Int(IntAVQuest0); got != 1 {
		t.Fatalf("value after a failed exchange = %d, want 1", got)
	}
}
