package admin

import (
	"sync"
	"testing"
)

// Replace is AdminData.reload: every check after it reads the new tables,
// and a check running beside it reads one table or the other.
func TestDataReplaceSwapsBothTables(t *testing.T) {
	old, err := NewData([]AccessLevel{{Level: 0}, {Level: 7, IsGM: true}}, []Command{{Name: "admin_reload", AccessLevel: 7}})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := NewData([]AccessLevel{{Level: 0}, {Level: 8, IsGM: true}}, []Command{{Name: "admin_reload", AccessLevel: 8}, {Name: "admin_kick", AccessLevel: 8}})
	if err != nil {
		t.Fatal(err)
	}
	gm := AccessLevel{Level: 7, IsGM: true}
	if !old.HasAccess("admin_reload", gm) {
		t.Fatal("level 7 cannot reload before the replace")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			old.HasAccess("admin_reload", gm)
			old.Defines("admin_kick")
			old.Resolve(7)
			old.MasterLevel()
		}
	}()
	old.Replace(fresh)
	wg.Wait()

	if old.HasAccess("admin_reload", gm) {
		t.Fatal("level 7 still reloads after the replace")
	}
	if !old.Defines("admin_kick") || old.CommandCount() != 2 || old.MasterLevel() != 8 || old.DefinesLevel(7) {
		t.Fatalf("replaced data: kick %v, commands %d, master %d, level 7 %v", old.Defines("admin_kick"), old.CommandCount(), old.MasterLevel(), old.DefinesLevel(7))
	}
}
