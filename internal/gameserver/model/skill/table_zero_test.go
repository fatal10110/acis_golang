package skill

import "testing"

// TestZeroTableReadsEmpty requires a zero-value Table to answer every read as
// an empty table, and to accept a Replace, instead of panicking.
func TestZeroTableReadsEmpty(t *testing.T) {
	var table Table

	if d, ok := table.Get(1, 1); ok || d.ID != 0 {
		t.Fatalf("Get on zero Table = %+v, %v; want zero, false", d, ok)
	}
	if got := table.MaxLevel(1); got != 0 {
		t.Fatalf("MaxLevel on zero Table = %d; want 0", got)
	}
	if got := table.Len(); got != 0 {
		t.Fatalf("Len on zero Table = %d; want 0", got)
	}
	if got := table.All(); len(got) != 0 {
		t.Fatalf("All on zero Table = %+v; want empty", got)
	}
	if got := table.TopLevels(); len(got) != 0 {
		t.Fatalf("TopLevels on zero Table = %+v; want empty", got)
	}

	table.Replace(NewTable([]Definition{{ID: 1, Level: 2, Name: "New"}}))
	if got := table.MaxLevel(1); got != 2 {
		t.Fatalf("MaxLevel after Replace = %d; want 2", got)
	}

	fresh := NewTable([]Definition{{ID: 1, Level: 1}})
	fresh.Replace(&Table{})
	if got := fresh.Len(); got != 0 {
		t.Fatalf("Len after Replace with zero Table = %d; want 0", got)
	}
}
