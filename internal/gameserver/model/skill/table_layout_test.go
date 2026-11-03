package skill

import "testing"

// TestTableKeepsLastDuplicateAndOrders requires a later entry at the same
// id and level to win, All to read in id-then-level order whatever the input
// order, and TopLevels to skip an id that has only enchant levels.
func TestTableKeepsLastDuplicateAndOrders(t *testing.T) {
	table := NewTable([]Definition{
		{ID: 7, Level: 2, Name: "a"},
		{ID: 3, Level: 101, Name: "enchant only"},
		{ID: 7, Level: 1, Name: "first"},
		{ID: 5, Level: 1, Name: "b"},
		{ID: 7, Level: 101, Name: "enchant"},
		{ID: 7, Level: 1, Name: "second"},
		{ID: 7, Level: 1, Name: "last"},
	})

	if got := table.Len(); got != 5 {
		t.Fatalf("Len = %d, want 5 (duplicates collapse)", got)
	}
	if d, ok := table.Get(7, 1); !ok || d.Name != "last" {
		t.Fatalf("Get(7, 1) = %+v, %v; want the last duplicate", d, ok)
	}
	if d, ok := table.Get(7, 3); ok || d != nil {
		t.Fatalf("Get(7, 3) = %+v, %v; want nil, false", d, ok)
	}

	want := []Ref{{3, 101}, {5, 1}, {7, 1}, {7, 2}, {7, 101}}
	all := table.All()
	if len(all) != len(want) {
		t.Fatalf("All = %d entries, want %d", len(all), len(want))
	}
	for i, def := range all {
		if got := (Ref{ID: def.ID, Level: def.Level}); got != want[i] {
			t.Fatalf("All[%d] = %+v, want %+v", i, got, want[i])
		}
	}

	top := table.TopLevels()
	if len(top) != 2 || top[0].ID != 5 || top[1].ID != 7 || top[1].Level != 2 {
		t.Fatalf("TopLevels = %+v; want id 5 level 1, id 7 level 2", top)
	}
	if got := table.MaxLevel(3); got != 0 {
		t.Fatalf("MaxLevel(3) = %d, want 0 (enchant levels stay out)", got)
	}
}

// TestTableHandsOutOneSharedDefinition requires every lookup of one id and
// level to reach the same stored definition, a copy from All not to reach
// it, and a definition read before a Replace to stay as it was read.
func TestTableHandsOutOneSharedDefinition(t *testing.T) {
	input := []Definition{{ID: 1, Level: 1, Name: "Old"}}
	table := NewTable(input)
	input[0].Name = "changed after NewTable"

	first, _ := table.Get(1, 1)
	second, _ := table.Definition(Ref{ID: 1, Level: 1})
	if first != second {
		t.Fatal("two lookups of one id and level returned different definitions")
	}
	if first.Name != "Old" {
		t.Fatalf("Name = %q; NewTable must not alias its input", first.Name)
	}
	all := table.All()
	all[0].Name = "changed copy"
	if first.Name != "Old" {
		t.Fatalf("Name = %q; All must return copies", first.Name)
	}

	table.Replace(NewTable([]Definition{{ID: 1, Level: 1, Name: "New"}}))
	if first.Name != "Old" {
		t.Fatalf("held definition Name = %q after Replace, want Old", first.Name)
	}
	if d, _ := table.Get(1, 1); d.Name != "New" {
		t.Fatalf("Get after Replace = %q, want New", d.Name)
	}
}
