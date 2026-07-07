package skill

import "testing"

func TestTable(t *testing.T) {
	table := NewTable([]Definition{
		{ID: 1, Level: 1},
		{ID: 1, Level: 2},
		{ID: 1, Level: 101}, // enchant level, excluded from MaxLevel
		{ID: 2, Level: 1},
	})

	if got := table.Len(); got != 4 {
		t.Fatalf("Len() = %d, want 4", got)
	}

	if d, ok := table.Get(1, 2); !ok || d.Level != 2 {
		t.Fatalf("Get(1, 2) = %+v, %v", d, ok)
	}
	if _, ok := table.Get(1, 3); ok {
		t.Fatal("Get(1, 3) ok = true, want false")
	}
	if _, ok := table.Get(99, 1); ok {
		t.Fatal("Get(99, 1) ok = true, want false")
	}

	if got := table.MaxLevel(1); got != 2 {
		t.Fatalf("MaxLevel(1) = %d, want 2 (enchant level 101 excluded)", got)
	}
	if got := table.MaxLevel(99); got != 0 {
		t.Fatalf("MaxLevel(99) = %d, want 0 for an unloaded id", got)
	}
}

func TestNewTableLaterEntryOverwrites(t *testing.T) {
	table := NewTable([]Definition{
		{ID: 1, Level: 1, Name: "first"},
		{ID: 1, Level: 1, Name: "second"},
	})
	if got := table.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	d, ok := table.Get(1, 1)
	if !ok || d.Name != "second" {
		t.Fatalf("Get(1, 1) = %+v, %v, want Name=second", d, ok)
	}
}
