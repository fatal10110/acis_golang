package player

import (
	"sort"
	"testing"
)

func TestLevelTable_Levels(t *testing.T) {
	table, err := NewLevelTable(map[int]Level{
		3: {RequiredExpToLevelUp: 300},
		1: {RequiredExpToLevelUp: 100},
		2: {RequiredExpToLevelUp: 200},
	})
	if err != nil {
		t.Fatalf("NewLevelTable() error: %v", err)
	}

	levels := table.Levels()
	if len(levels) != table.Count() {
		t.Fatalf("Levels() returned %d entries, Count() = %d", len(levels), table.Count())
	}
	if !sort.IntsAreSorted(levels) {
		t.Fatalf("Levels() not sorted ascending: %v", levels)
	}
	if want := []int{1, 2, 3}; !equalInts(levels, want) {
		t.Fatalf("Levels() = %v, want %v", levels, want)
	}
}
