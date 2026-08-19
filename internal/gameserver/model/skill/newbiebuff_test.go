package skill

import (
	"testing"
)

func TestNewNewbieBuff(t *testing.T) {
	got := NewNewbieBuff(4322, 1, 8, 24, false)
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
