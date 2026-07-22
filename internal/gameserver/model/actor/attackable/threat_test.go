package attackable

import (
	"sync"
	"testing"
)

// Clamp fixtures below were cross-checked against the accumulation formula
// (min(current+delta, 999999999), no lower bound) by running it in a small
// standalone program: 999999998+10 and 500000000+600000000 both clamp to
// 999999999; 5+(-100) is left at -95 (hate has no floor).

func TestThreatTable_AddDamageClampsAtMax(t *testing.T) {
	owner := combatant(1)
	attacker := combatant(2)
	table := NewThreatTable(owner)

	table.AddDamage(attacker, 999999998, 0)
	table.AddDamage(attacker, 10, 0)

	got, ok := table.Get(attacker)
	if !ok {
		t.Fatal("Get: attacker missing after AddDamage")
	}
	if got.Damage != 999999999 {
		t.Errorf("Damage = %v, want 999999999 (clamped)", got.Damage)
	}

	hateOwner := combatant(3)
	hateAttacker := combatant(4)
	hateTable := NewThreatTable(hateOwner)
	hateTable.AddDamage(hateAttacker, 0, 500000000)
	hateTable.AddDamage(hateAttacker, 0, 600000000)

	got, _ = hateTable.Get(hateAttacker)
	if got.Hate != 999999999 {
		t.Errorf("Hate = %v, want 999999999 (clamped)", got.Hate)
	}
}

func TestThreatTable_HateHasNoLowerClamp(t *testing.T) {
	table := NewThreatTable(combatant(1))
	attacker := combatant(2)

	table.AddDamage(attacker, 0, 5)
	table.ReduceAllHate(100)

	got, _ := table.Get(attacker)
	if got.Hate != -95 {
		t.Errorf("Hate = %v, want -95 (no floor)", got.Hate)
	}
}

func TestThreatTable_AddDamageIgnoresNilAttacker(t *testing.T) {
	table := NewThreatTable(combatant(1))
	table.AddDamage(nil, 100, 100)

	if !table.IsEmpty() {
		t.Error("table not empty after AddDamage(nil, ...)")
	}
}

func TestThreatTable_AddDamageSkipsSiegeGuardOnSiegeGuard(t *testing.T) {
	owner := &fakeCombatant{id: 1, siegeGuard: true}
	attacker := &fakeCombatant{id: 2, siegeGuard: true}
	table := NewThreatTable(owner)

	table.AddDamage(attacker, 100, 100)

	if !table.IsEmpty() {
		t.Error("table not empty after siege guard attacked siege guard")
	}
}

func TestThreatTable_AddDamageAllowsSiegeGuardOnNonGuard(t *testing.T) {
	owner := &fakeCombatant{id: 1, siegeGuard: true}
	attacker := combatant(2)
	table := NewThreatTable(owner)

	table.AddDamage(attacker, 100, 100)

	if table.IsEmpty() {
		t.Error("table empty after siege guard attacked by a non-guard")
	}
}

func TestThreatTable_MostHated(t *testing.T) {
	// Fixture: entries {a:10, b:25, c:-5, d:0}; filtering hate>0 and
	// taking the max selects b (25). Cross-checked against a standalone
	// stream().filter(>0).max(...) run in Java.
	owner := combatant(1)
	table := NewThreatTable(owner)

	a, b, c, d := combatant(10), combatant(11), combatant(12), combatant(13)
	table.AddDamage(a, 0, 10)
	table.AddDamage(b, 0, 25)
	table.AddDamage(c, 0, -5)
	table.AddDamage(d, 0, 0)

	got, ok := table.MostHated()
	if !ok {
		t.Fatal("MostHated: ok = false, want true")
	}
	if got.Attacker.ObjectID() != b.ObjectID() {
		t.Errorf("MostHated attacker = %d, want %d", got.Attacker.ObjectID(), b.ObjectID())
	}
	if got.Hate != 25 {
		t.Errorf("MostHated hate = %v, want 25", got.Hate)
	}
}

func TestThreatTable_MostHatedNoneWhenAllNonPositive(t *testing.T) {
	// Fixture: entries {a:-5, b:0}; filtering hate>0 yields nothing.
	table := NewThreatTable(combatant(1))
	a, b := combatant(10), combatant(11)
	table.AddDamage(a, 0, -5)
	table.AddDamage(b, 0, 0)

	if _, ok := table.MostHated(); ok {
		t.Error("MostHated: ok = true, want false when no entry has positive hate")
	}
}

func TestThreatTable_MostHatedNoneWhenOwnerAlikeDead(t *testing.T) {
	owner := &fakeCombatant{id: 1, alikeDead: true}
	table := NewThreatTable(owner)
	table.AddDamage(combatant(2), 0, 100)

	if _, ok := table.MostHated(); ok {
		t.Error("MostHated: ok = true, want false while owner is alike dead")
	}
}

func TestThreatTable_MostHatedNoneWhenEmpty(t *testing.T) {
	table := NewThreatTable(combatant(1))
	if _, ok := table.MostHated(); ok {
		t.Error("MostHated: ok = true, want false on an empty table")
	}
}

func TestThreatTable_StopHateKeepsEntry(t *testing.T) {
	table := NewThreatTable(combatant(1))
	attacker := combatant(2)
	table.AddDamage(attacker, 42, 100)

	table.StopHate(attacker)

	got, ok := table.Get(attacker)
	if !ok {
		t.Fatal("Get: entry dropped by StopHate, want it kept")
	}
	if got.Hate != 0 {
		t.Errorf("Hate = %v, want 0 after StopHate", got.Hate)
	}
	if got.Damage != 42 {
		t.Errorf("Damage = %v, want 42 preserved after StopHate", got.Damage)
	}
}

func TestThreatTable_StopHateOnUnknownTargetIsNoop(t *testing.T) {
	table := NewThreatTable(combatant(1))
	table.StopHate(combatant(99)) // must not panic
}

func TestThreatTable_RefreshStopsDeadThreatAndRemovesLostThreat(t *testing.T) {
	table := NewThreatTable(combatant(1))
	dead := &fakeCombatant{id: 2, alikeDead: true}
	lost := combatant(3)
	kept := combatant(4)
	table.AddDamage(dead, 40, 100)
	table.AddDamage(lost, 50, 110)
	table.AddDamage(kept, 60, 120)

	changed := table.Refresh(func(c Combatant) bool {
		return c.ObjectID() != lost.ObjectID()
	})

	if len(changed) != 2 {
		t.Fatalf("Refresh changed %d targets, want 2", len(changed))
	}
	got, ok := table.Get(dead)
	if !ok {
		t.Fatal("dead attacker entry was dropped, want it kept for damage accounting")
	}
	if got.Hate != 0 || got.Damage != 40 {
		t.Fatalf("dead attacker entry = %+v, want hate zeroed and damage preserved", got)
	}
	if _, ok := table.Get(lost); ok {
		t.Fatal("lost attacker entry still present after Refresh")
	}
	if got, ok := table.Get(kept); !ok || got.Hate != 120 {
		t.Fatalf("kept attacker entry = (%+v, %v), want present with hate 120", got, ok)
	}
}

func TestThreatTable_Remove(t *testing.T) {
	table := NewThreatTable(combatant(1))
	attacker := combatant(2)
	table.AddDamage(attacker, 42, 100)

	table.Remove(attacker)

	if _, ok := table.Get(attacker); ok {
		t.Error("Get: entry present after Remove")
	}
}

func TestThreatTable_ZeroHateKeepsEntries(t *testing.T) {
	table := NewThreatTable(combatant(1))
	a, b := combatant(2), combatant(3)
	table.AddDamage(a, 10, 10)
	table.AddDamage(b, 20, 20)

	table.ZeroHate()

	if len(table.Snapshot()) != 2 {
		t.Fatalf("Snapshot has %d entries after ZeroHate, want 2", len(table.Snapshot()))
	}
	for _, e := range table.Snapshot() {
		if e.Hate != 0 {
			t.Errorf("entry %d hate = %v, want 0", e.Attacker.ObjectID(), e.Hate)
		}
	}
}

func TestThreatTable_ClearDropsEntries(t *testing.T) {
	table := NewThreatTable(combatant(1))
	table.AddDamage(combatant(2), 10, 10)

	table.Clear()

	if !table.IsEmpty() {
		t.Error("table not empty after Clear")
	}
}

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
