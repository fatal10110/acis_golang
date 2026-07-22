package attackable

import (
	"sync"
	"testing"
)

func TestHateTable_AddIsUncapped(t *testing.T) {
	// Fixture: 999999999 + 999999999 = 1999999998, cross-checked in a
	// standalone Java run of the same accumulation (plain double sum, no
	// clamp) — HateTable, unlike ThreatTable, never caps hate.
	table := NewHateTable(combatant(1))
	attacker := combatant(2)

	table.Add(attacker, 999999999)
	table.Add(attacker, 999999999)

	if got := table.Hate(attacker); got != 1999999998 {
		t.Errorf("Hate = %v, want 1999999998 (uncapped)", got)
	}
}

func TestHateTable_AddIgnoresNilAttacker(t *testing.T) {
	table := NewHateTable(combatant(1))
	table.Add(nil, 100)

	if !table.IsEmpty() {
		t.Error("table not empty after Add(nil, ...)")
	}
}

func TestHateTable_AddSkipsSiegeGuardOnSiegeGuard(t *testing.T) {
	owner := &fakeCombatant{id: 1, siegeGuard: true}
	attacker := &fakeCombatant{id: 2, siegeGuard: true}
	table := NewHateTable(owner)

	table.Add(attacker, 100)

	if !table.IsEmpty() {
		t.Error("table not empty after siege guard attacked siege guard")
	}
}

func TestHateTable_AddDefaultUsesOpeningTerritoryHate(t *testing.T) {
	table := NewHateTable(combatant(1))
	first := combatant(2)
	second := combatant(3)

	table.AddDefault(first, true)
	table.AddDefault(second, true)

	if got := table.Hate(first); got != 300 {
		t.Fatalf("first default hate in territory = %v, want 300", got)
	}
	if got := table.Hate(second); got != 100 {
		t.Fatalf("second default hate in territory = %v, want 100", got)
	}
}

func TestHateTable_AddDefaultOutsideTerritoryUsesBaseHate(t *testing.T) {
	table := NewHateTable(combatant(1))
	attacker := combatant(2)

	table.AddDefault(attacker, false)

	if got := table.Hate(attacker); got != 100 {
		t.Fatalf("default hate outside territory = %v, want 100", got)
	}
}

func TestHateTable_MostHatedHasNoPositiveFilter(t *testing.T) {
	// Fixture: entries {a:-5, b:-10}; Collections.max with no filter picks
	// a (-5), the greatest even though every entry is negative. Cross-
	// checked in a standalone Java run — this is the key divergence from
	// ThreatTable.MostHated, which would report none found here.
	table := NewHateTable(combatant(1))
	a, b := combatant(10), combatant(11)
	table.Add(a, -5)
	table.Add(b, -10)

	got, ok := table.MostHated()
	if !ok {
		t.Fatal("MostHated: ok = false, want true even when every entry is negative")
	}
	if got.Attacker.ObjectID() != a.ObjectID() {
		t.Errorf("MostHated attacker = %d, want %d", got.Attacker.ObjectID(), a.ObjectID())
	}
	if got.Hate != -5 {
		t.Errorf("MostHated hate = %v, want -5", got.Hate)
	}
}

func TestHateTable_MostHatedNoneWhenOwnerAlikeDead(t *testing.T) {
	owner := &fakeCombatant{id: 1, alikeDead: true}
	table := NewHateTable(owner)
	table.Add(combatant(2), 100)

	if _, ok := table.MostHated(); ok {
		t.Error("MostHated: ok = true, want false while owner is alike dead")
	}
}

func TestHateTable_MostHatedNoneWhenEmpty(t *testing.T) {
	table := NewHateTable(combatant(1))
	if _, ok := table.MostHated(); ok {
		t.Error("MostHated: ok = true, want false on an empty table")
	}
}

func TestHateTable_StopHateDropsEntry(t *testing.T) {
	// Unlike ThreatTable.StopHate (zeroes, keeps entry), HateTable.StopHate
	// removes the entry entirely.
	table := NewHateTable(combatant(1))
	attacker := combatant(2)
	table.Add(attacker, 100)

	table.StopHate(attacker)

	if len(table.Snapshot()) != 0 {
		t.Errorf("Snapshot has %d entries after StopHate, want 0 (entry dropped)", len(table.Snapshot()))
	}
	if got := table.Hate(attacker); got != 0 {
		t.Errorf("Hate = %v after StopHate, want 0", got)
	}
}

func TestHateTable_RefreshDropsDeadAndLostEntries(t *testing.T) {
	table := NewHateTable(combatant(1))
	dead := &fakeCombatant{id: 2, alikeDead: true}
	lost := combatant(3)
	kept := combatant(4)
	table.Add(dead, 100)
	table.Add(lost, 110)
	table.Add(kept, 120)

	changed := table.Refresh(func(c Combatant) bool {
		return c.ObjectID() != lost.ObjectID()
	})

	if len(changed) != 2 {
		t.Fatalf("Refresh changed %d targets, want 2", len(changed))
	}
	if got := table.Hate(dead); got != 0 {
		t.Fatalf("dead attacker hate = %v, want removed", got)
	}
	if got := table.Hate(lost); got != 0 {
		t.Fatalf("lost attacker hate = %v, want removed", got)
	}
	if got := table.Hate(kept); got != 120 {
		t.Fatalf("kept attacker hate = %v, want 120", got)
	}
}

func TestHateTable_ReduceAllHateHasNoFloor(t *testing.T) {
	table := NewHateTable(combatant(1))
	attacker := combatant(2)
	table.Add(attacker, 5)

	table.ReduceAllHate(100)

	if got := table.Hate(attacker); got != -95 {
		t.Errorf("Hate = %v, want -95 (no floor)", got)
	}
	if len(table.Snapshot()) != 1 {
		t.Error("ReduceAllHate must not drop entries")
	}
}

func TestHateTable_ZeroHateKeepsEntries(t *testing.T) {
	table := NewHateTable(combatant(1))
	attacker := combatant(2)
	table.Add(attacker, 100)

	table.ZeroHate()

	if got := table.Hate(attacker); got != 0 {
		t.Errorf("Hate = %v, want 0 after ZeroHate", got)
	}
	if len(table.Snapshot()) != 1 {
		t.Error("ZeroHate must not drop entries")
	}
}

func TestHateTable_ClearDropsEntries(t *testing.T) {
	table := NewHateTable(combatant(1))
	table.Add(combatant(2), 10)

	table.Clear()

	if !table.IsEmpty() {
		t.Error("table not empty after Clear")
	}
}

func TestHateTable_ConcurrentAccess(t *testing.T) {
	owner := combatant(1)
	table := NewHateTable(owner)

	var wg sync.WaitGroup
	for i := int32(0); i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			attacker := combatant(id)
			table.Add(attacker, 10)
			table.Hate(attacker)
			table.MostHated()
			table.Snapshot()
			table.ReduceAllHate(1)
			table.StopHate(attacker)
		}(i)
	}
	wg.Wait()

	if !table.IsEmpty() {
		t.Errorf("table has %d entries after concurrent add/remove, want 0", len(table.Snapshot()))
	}
}
