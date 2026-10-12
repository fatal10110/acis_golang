package skill

import (
	"sync"
	"testing"
)

// TestTableReplaceSwapsDefinitionsAndMaxLevel requires Replace to hand
// every holder of the table the new definitions together with their
// max-level index, and a reader racing it to see one set or the other.
func TestTableReplaceSwapsDefinitionsAndMaxLevel(t *testing.T) {
	old := []Definition{{ID: 1, Level: 1, Name: "Old"}, {ID: 1, Level: 2, Name: "Old"}, {ID: 2, Level: 1, Name: "Gone"}}
	fresh := []Definition{{ID: 1, Level: 1, Name: "New"}, {ID: 1, Level: 2, Name: "New"}, {ID: 1, Level: 3, Name: "New"}, {ID: 1, Level: 101, Name: "New"}}
	table := NewTable(old)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			d, ok := table.Get(1, 1)
			if !ok || d.Name != "Old" && d.Name != "New" {
				t.Errorf("Get(1, 1) during Replace = %+v, %v", d, ok)
				return
			}
			if top := table.TopLevels(); len(top) == 0 || top[0].ID != 1 {
				t.Errorf("TopLevels during Replace = %+v", top)
				return
			}
		}
	}()
	table.Replace(NewTable(fresh))
	wg.Wait()

	if d, ok := table.Get(1, 3); !ok || d.Name != "New" {
		t.Fatalf("Get(1, 3) = %+v, %v; want the replaced definition", d, ok)
	}
	if _, ok := table.Get(2, 1); ok {
		t.Fatal("Get(2, 1) still finds a definition the replacement dropped")
	}
	if got := table.MaxLevel(1); got != 3 {
		t.Fatalf("MaxLevel(1) = %d, want 3 (the enchant level stays out)", got)
	}
	if got := table.Len(); got != len(fresh) {
		t.Fatalf("Len = %d, want %d", got, len(fresh))
	}
}
