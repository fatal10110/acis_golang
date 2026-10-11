package attackable

import "testing"

func TestHateTable_RemoveIfDropsMatchingEntries(t *testing.T) {
	table := NewHateTable(combatant(1))
	far, near := combatant(2), combatant(3)
	table.Add(far, 100)
	table.Add(near, 50)

	removed := table.RemoveIf(func(c Combatant) bool { return c.ObjectID() == far.ObjectID() })

	if len(removed) != 1 || removed[0].ObjectID() != far.ObjectID() {
		t.Fatalf("RemoveIf removed %v, want only the matching attacker", removed)
	}
	if got := table.Hate(far); got != 0 {
		t.Fatalf("removed attacker hate = %v, want dropped", got)
	}
	if got := table.Hate(near); got != 50 {
		t.Fatalf("kept attacker hate = %v, want 50", got)
	}
}
