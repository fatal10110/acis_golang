package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestNewBufferSkillDefaultsLevelFromTable(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("id", "1035")
	set.Set("type", "Buffs")
	set.Set("desc", "desc")

	entry, err := NewBufferSkill(set, NewTable([]Definition{{ID: 1035, Level: 4}}))
	if err != nil {
		t.Fatalf("NewBufferSkill() error: %v", err)
	}
	if entry.Skill.ID != 1035 || entry.Skill.Level != 4 || entry.Price != 0 {
		t.Fatalf("NewBufferSkill() = %+v", entry)
	}
}

func TestNewBufferTablePreservesCategoryOrder(t *testing.T) {
	table, err := NewBufferTable([]BufferSkill{
		{Skill: Ref{ID: 1035, Level: 4}, Category: "Buffs"},
		{Skill: Ref{ID: 271, Level: 1}, Category: "Dances"},
		{Skill: Ref{ID: 264, Level: 1}, Category: "Songs"},
	})
	if err != nil {
		t.Fatalf("NewBufferTable() error: %v", err)
	}

	got := table.Categories()
	want := []string{"Buffs", "Dances", "Songs"}
	if len(got) != len(want) {
		t.Fatalf("Categories() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Categories()[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}
