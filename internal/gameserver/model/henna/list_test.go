package henna

import "testing"

func TestMaxSlots(t *testing.T) {
	if got := MaxSlots(0); got != 0 {
		t.Fatalf("MaxSlots(0) = %d, want 0", got)
	}
	if got := MaxSlots(1); got != 2 {
		t.Fatalf("MaxSlots(1) = %d, want 2", got)
	}
	if got := MaxSlots(2); got != 3 {
		t.Fatalf("MaxSlots(2) = %d, want 3", got)
	}
	if got := MaxSlots(3); got != 3 {
		t.Fatalf("MaxSlots(3) = %d, want 3", got)
	}
}

func TestListRestoreRecalculateAndCap(t *testing.T) {
	table := NewTable([]Henna{
		{SymbolID: 1, STR: 3, CON: -1, Classes: []int{2}},
		{SymbolID: 2, STR: 3, Classes: []int{2}},
		{SymbolID: 3, STR: 1, Classes: []int{0}}, // wrong class: ignored for stats
	})
	var list List
	list.Restore([]Row{
		{Slot: 1, SymbolID: 1},
		{Slot: 2, SymbolID: 2},
		{Slot: 3, SymbolID: 3},
		{Slot: 9, SymbolID: 1}, // invalid
	}, table.Find)
	list.Recalculate(2)
	if got := list.Stat(StatSTR); got != MaxStatValue {
		t.Fatalf("STR = %d, want capped %d", got, MaxStatValue)
	}
	if got := list.Stat(StatCON); got != -1 {
		t.Fatalf("CON = %d, want -1", got)
	}
	snap := list.Snapshot(2, 2)
	if snap.MaxSlots != 3 || len(snap.Equipped) != 3 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if snap.Equipped[2].ActiveSymbolID != 0 {
		t.Fatalf("ineligible dye active = %d, want 0", snap.Equipped[2].ActiveSymbolID)
	}
}

func TestListAddRemove(t *testing.T) {
	h := Henna{SymbolID: 7, INT: 1, MEN: -3, Classes: []int{11}}
	var list List
	slot, ok := list.Add(h, 11, 1)
	if !ok || slot != 1 {
		t.Fatalf("Add = (%d, %v), want (1, true)", slot, ok)
	}
	if list.Stat(StatINT) != 1 || list.Stat(StatMEN) != -3 {
		t.Fatalf("stats INT=%d MEN=%d", list.Stat(StatINT), list.Stat(StatMEN))
	}
	if !list.IsFull(1) {
		// class level 1 → 2 slots; one filled → not full
	} else {
		t.Fatal("IsFull after one add on classLevel 1")
	}
	_, ok = list.Add(h, 11, 1)
	if !ok {
		t.Fatal("second Add failed")
	}
	if !list.IsFull(1) {
		t.Fatal("want full after two adds on classLevel 1")
	}
	if _, ok := list.Add(h, 11, 1); ok {
		t.Fatal("third Add on classLevel 1 succeeded")
	}
	slot, ok = list.Remove(7, 11)
	if !ok || slot != 1 {
		t.Fatalf("Remove = (%d, %v), want (1, true)", slot, ok)
	}
	if _, ok := list.BySymbolID(7); !ok {
		t.Fatal("second slot henna missing after removing first")
	}
}
