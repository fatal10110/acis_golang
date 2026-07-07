package player

import (
	"sort"
	"testing"
)

func TestTemplateTable_All(t *testing.T) {
	// 0, 10 and 18 are base professions (classParent maps them to -1), so
	// NewTemplateTable needs no other entries to resolve them.
	table, err := NewTemplateTable(map[int]*Template{
		18: {ID: 18},
		0:  {ID: 0},
		10: {ID: 10},
	})
	if err != nil {
		t.Fatalf("NewTemplateTable() error: %v", err)
	}

	all := table.All()
	if len(all) != table.Count() {
		t.Fatalf("All() returned %d templates, Count() = %d", len(all), table.Count())
	}

	var ids []int
	for _, tpl := range all {
		ids = append(ids, tpl.ID)
	}
	if !sort.IntsAreSorted(ids) {
		t.Fatalf("All() not sorted ascending by ID: %v", ids)
	}
	if want := []int{0, 10, 18}; !equalInts(ids, want) {
		t.Fatalf("All() ids = %v, want %v", ids, want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
