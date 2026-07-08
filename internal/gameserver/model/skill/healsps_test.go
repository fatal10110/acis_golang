package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestNewHealSps(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("magicLevel", "76")
	set.Set("correction", "292")
	set.Set("neededMatk", "900")

	got, err := NewHealSps(set)
	if err != nil {
		t.Fatalf("NewHealSps() error: %v", err)
	}
	if got.MagicLevel != 76 || got.Correction != 292 || got.NeededMAtk != 900 {
		t.Fatalf("NewHealSps() = %+v", got)
	}

	set = commons.NewStatSet()
	set.Set("correction", "17")
	if _, err := NewHealSps(set); err == nil {
		t.Fatal("expected an error for missing neededMatk and selectors, got nil")
	}
}

func TestHealSpsTableCalculate(t *testing.T) {
	table, err := NewHealSpsTable([]HealSps{
		{MagicLevel: 74, Correction: 281, NeededMAtk: 850},
		{SkillID: 1401, SkillLevel: 11, Correction: 286, NeededMAtk: 875},
		{MagicLevel: 76, Correction: 292, NeededMAtk: 900},
	})
	if err != nil {
		t.Fatalf("NewHealSpsTable() error: %v", err)
	}

	if got := table.Calculate(1401, 11, 76, 875); got != 286 {
		t.Fatalf("Calculate(skill match) = %v, want 286", got)
	}
	if got := table.Calculate(2000, 1, 76, 890); got != 287 {
		t.Fatalf("Calculate(magic level fallback) = %v, want 287", got)
	}
	if got := table.Calculate(2000, 1, 1, 1); got != 0 {
		t.Fatalf("Calculate(no match) = %v, want 0", got)
	}
}
