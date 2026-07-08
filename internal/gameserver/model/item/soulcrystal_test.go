package item

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestNewSoulCrystal(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("level", "12")
	set.Set("initial", "5582")
	set.Set("staged", "5914")
	set.Set("broken", "4664")

	got, err := NewSoulCrystal(set)
	if err != nil {
		t.Fatalf("NewSoulCrystal() error: %v", err)
	}

	want := SoulCrystal{Level: 12, InitialItemID: 5582, StagedItemID: 5914, BrokenItemID: 4664}
	if got != want {
		t.Fatalf("NewSoulCrystal() = %+v, want %+v", got, want)
	}

	set = commons.NewStatSet()
	set.Set("initial", "4629")
	if _, err := NewSoulCrystal(set); err == nil {
		t.Fatal("expected an error for missing staged/broken/level, got nil")
	}
}

func TestNewSoulCrystalLevelingInfo(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("id", "22215")
	set.Set("chanceStage", "100")
	set.Set("chanceBreak", "0")
	set.Set("skill", "false")
	set.Set("absorbType", "PARTY_ONE_RANDOM")
	set.Set("levelList", "10;11")

	got, err := NewSoulCrystalLevelingInfo(set)
	if err != nil {
		t.Fatalf("NewSoulCrystalLevelingInfo() error: %v", err)
	}

	if got.NPCID != 22215 || got.ChanceStage != 100 || got.ChanceBreak != 0 || got.SkillRequired || got.AbsorbType != "PARTY_ONE_RANDOM" {
		t.Fatalf("NewSoulCrystalLevelingInfo() = %+v", got)
	}
	if want := []int{10, 11}; !reflect.DeepEqual(got.Levels, want) {
		t.Fatalf("Levels = %#v, want %#v", got.Levels, want)
	}

	set = commons.NewStatSet()
	set.Set("id", "1")
	if _, err := NewSoulCrystalLevelingInfo(set); err == nil {
		t.Fatal("expected an error for missing attributes, got nil")
	}
}
