package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestNewNewbieBuff(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("skillId", "4322")
	set.Set("skillLevel", "1")
	set.Set("lowerLevel", "8")
	set.Set("upperLevel", "24")
	set.Set("isMagicClass", "false")

	got, err := NewNewbieBuff(set)
	if err != nil {
		t.Fatalf("NewNewbieBuff() error: %v", err)
	}
	if got.Skill.ID != 4322 || got.Skill.Level != 1 || got.LowerLevel != 8 || got.UpperLevel != 24 || got.IsMagicClass {
		t.Fatalf("NewNewbieBuff() = %+v", got)
	}
}

func TestNewbieBuffTableQueries(t *testing.T) {
	table := NewNewbieBuffTable([]NewbieBuff{
		{Skill: Ref{ID: 4322, Level: 1}, LowerLevel: 8, UpperLevel: 24, IsMagicClass: false},
		{Skill: Ref{ID: 4323, Level: 1}, LowerLevel: 11, UpperLevel: 23, IsMagicClass: false},
		{Skill: Ref{ID: 4322, Level: 1}, LowerLevel: 8, UpperLevel: 24, IsMagicClass: true},
	})

	if got := table.LowestBuffLevel(false); got != 8 {
		t.Fatalf("LowestBuffLevel(false) = %d, want 8", got)
	}
	if got := table.LowestBuffLevel(true); got != 8 {
		t.Fatalf("LowestBuffLevel(true) = %d, want 8", got)
	}

	phys := table.ValidBuffs(false, 12)
	if len(phys) != 2 {
		t.Fatalf("len(ValidBuffs(false, 12)) = %d, want 2", len(phys))
	}
	mage := table.ValidBuffs(true, 12)
	if len(mage) != 1 || mage[0].Skill.ID != 4322 {
		t.Fatalf("ValidBuffs(true, 12) = %+v", mage)
	}
}
