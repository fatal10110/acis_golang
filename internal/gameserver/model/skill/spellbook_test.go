package skill

import (
	"testing"
)

func TestNewSpellbook(t *testing.T) {
	got := NewSpellbook(2, 1512)
	want := Spellbook{SkillID: 2, ItemID: 1512}
	if got != want {
		t.Fatalf("NewSpellbook() = %+v, want %+v", got, want)
	}
}

func TestSpellbookTableBookForSkill(t *testing.T) {
	table, err := NewSpellbookTable([]Spellbook{{SkillID: 2, ItemID: 1512}})
	if err != nil {
		t.Fatalf("NewSpellbookTable() error: %v", err)
	}

	if got := table.BookForSkill(2, 1, true, true); got != 1512 {
		t.Fatalf("BookForSkill(2, 1, true, true) = %d, want 1512", got)
	}
	if got := table.BookForSkill(2, 2, true, true); got != 0 {
		t.Fatalf("BookForSkill(2, 2, true, true) = %d, want 0", got)
	}
	if got := table.BookForSkill(DivineInspirationSkillID, 3, true, true); got != 8620 {
		t.Fatalf("BookForSkill(divine inspiration, 3, true, true) = %d, want 8620", got)
	}
	if got := table.BookForSkill(DivineInspirationSkillID, 3, true, false); got != 0 {
		t.Fatalf("BookForSkill(divine inspiration, 3, true, false) = %d, want 0", got)
	}
	if got := table.BookForSkill(2, 1, false, true); got != 0 {
		t.Fatalf("BookForSkill(2, 1, false, true) = %d, want 0", got)
	}
}
