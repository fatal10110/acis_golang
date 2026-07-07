package npc

import (
	"sort"
	"testing"
)

func TestTable_All(t *testing.T) {
	table := NewTable([]*Template{
		{ID: 30, Name: "c"},
		{ID: 10, Name: "a"},
		{ID: 20, Name: "b"},
	})

	all := table.All()
	if len(all) != table.Len() {
		t.Fatalf("All() returned %d templates, Len() = %d", len(all), table.Len())
	}

	var ids []int
	for _, tpl := range all {
		ids = append(ids, tpl.ID)
	}
	if !sort.IntsAreSorted(ids) {
		t.Fatalf("All() not sorted ascending by ID: %v", ids)
	}
	if ids[0] != 10 || ids[len(ids)-1] != 30 {
		t.Fatalf("All() ids = %v, want [10 20 30]", ids)
	}
}
