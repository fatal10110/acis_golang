package attackable

import (
	"sync"
	"testing"
)

func TestThreatTable_ConcurrentAccess(t *testing.T) {
	owner := combatant(1)
	table := NewThreatTable(owner)

	var wg sync.WaitGroup
	for i := int32(0); i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			attacker := combatant(id)
			table.AddDamage(attacker, 10, 10)
			table.Hate(attacker)
			table.Get(attacker)
			table.MostHated()
			table.Snapshot()
			table.ReduceAllHate(1)
			table.StopHate(attacker)
			table.Remove(attacker)
		}(i)
	}
	wg.Wait()

	if !table.IsEmpty() {
		t.Errorf("table has %d entries after concurrent add/remove, want 0", len(table.Snapshot()))
	}
}
