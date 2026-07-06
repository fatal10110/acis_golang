package item

import "testing"

func TestSlot_PaperdollIndex(t *testing.T) {
	tests := []struct {
		slot Slot
		want int
	}{
		{SlotUnderwear, 0},
		{SlotLEar, 1},
		{SlotREar, 2},
		{SlotNeck, 3},
		{SlotLFinger, 4},
		{SlotRFinger, 5},
		{SlotHead, 6},
		{SlotRHand, 7},
		{SlotLRHand, 7},
		{SlotLHand, 8},
		{SlotGloves, 9},
		{SlotChest, 10},
		{SlotFullArmor, 10},
		{SlotAllDress, 10},
		{SlotLegs, 11},
		{SlotFeet, 12},
		{SlotBack, 13},
		{SlotFace, 14},
		{SlotHairAll, 14},
		{SlotHair, 15},
	}
	for _, tt := range tests {
		got, ok := tt.slot.PaperdollIndex()
		if !ok {
			t.Errorf("Slot(%d).PaperdollIndex() reported no position, want %d", tt.slot, tt.want)
			continue
		}
		if got != tt.want {
			t.Errorf("Slot(%d).PaperdollIndex() = %d, want %d", tt.slot, got, tt.want)
		}
	}
}

func TestSlot_PaperdollIndex_PairedSlotsUnresolved(t *testing.T) {
	for _, s := range []Slot{SlotNone, SlotLREar, SlotLRFinger, SlotWolf} {
		if _, ok := s.PaperdollIndex(); ok {
			t.Errorf("Slot(%d).PaperdollIndex() reported a position, want none", s)
		}
	}
}
