package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestNewFishingSkill(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("id", "1312")
	set.Set("lvl", "1")
	set.Set("minLvl", "1")
	set.Set("itemId", "57")
	set.Set("itemCount", "1000")

	got, err := NewFishingSkill(set)
	if err != nil {
		t.Fatalf("NewFishingSkill() error: %v", err)
	}
	want := FishingSkill{ID: 1312, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1000, Dwarven: false}
	if got != want {
		t.Fatalf("NewFishingSkill() = %+v, want %+v", got, want)
	}

	set.Set("isDwarven", "true")
	got, err = NewFishingSkill(set)
	if err != nil {
		t.Fatalf("NewFishingSkill() error: %v", err)
	}
	if !got.Dwarven {
		t.Fatal("NewFishingSkill() Dwarven = false, want true")
	}

	t.Run("missing required attribute", func(t *testing.T) {
		s := commons.NewStatSet()
		s.Set("id", "1")
		s.Set("lvl", "1")
		s.Set("minLvl", "1")
		if _, err := NewFishingSkill(s); err == nil {
			t.Fatal("expected an error for missing itemId/itemCount, got nil")
		}
	})
}

func TestNewClanSkill(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("id", "370")
	set.Set("lvl", "1")
	set.Set("minLvl", "5")
	set.Set("cost", "500")
	set.Set("itemId", "8166")

	got, err := NewClanSkill(set)
	if err != nil {
		t.Fatalf("NewClanSkill() error: %v", err)
	}
	want := ClanSkill{ID: 370, Level: 1, MinLevel: 5, Cost: 500, ItemID: 8166}
	if got != want {
		t.Fatalf("NewClanSkill() = %+v, want %+v", got, want)
	}

	t.Run("missing required attribute", func(t *testing.T) {
		s := commons.NewStatSet()
		s.Set("id", "1")
		s.Set("lvl", "1")
		if _, err := NewClanSkill(s); err == nil {
			t.Fatal("expected an error for missing minLvl/cost/itemId, got nil")
		}
	})
}

func TestNewEnchantSkill(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("id", "1")
	set.Set("lvl", "101")
	set.Set("exp", "5500000")
	set.Set("sp", "550000")
	set.Set("rate76", "82")
	set.Set("rate77", "92")
	set.Set("rate78", "97")
	set.Set("rate79", "100")
	set.Set("rate80", "100")
	set.Set("itemNeeded", "6622-1")

	got, err := NewEnchantSkill(set)
	if err != nil {
		t.Fatalf("NewEnchantSkill() error: %v", err)
	}
	want := EnchantSkill{
		ID: 1, Level: 101, Exp: 5500000, SP: 550000,
		Rate76: 82, Rate77: 92, Rate78: 97, Rate79: 100, Rate80: 100,
		ItemID: 6622, ItemCount: 1,
	}
	if got != want {
		t.Fatalf("NewEnchantSkill() = %+v, want %+v", got, want)
	}

	t.Run("no item requirement", func(t *testing.T) {
		set.Unset("itemNeeded")
		got, err := NewEnchantSkill(set)
		if err != nil {
			t.Fatalf("NewEnchantSkill() error: %v", err)
		}
		if got.ItemID != 0 || got.ItemCount != 0 {
			t.Fatalf("NewEnchantSkill() item = %d/%d, want 0/0", got.ItemID, got.ItemCount)
		}
	})

	t.Run("missing a required rate", func(t *testing.T) {
		s := commons.NewStatSet()
		s.Set("id", "1")
		s.Set("lvl", "101")
		s.Set("exp", "1")
		s.Set("sp", "1")
		s.Set("rate76", "1")
		s.Set("rate77", "1")
		s.Set("rate78", "1")
		s.Set("rate79", "1")
		if _, err := NewEnchantSkill(s); err == nil {
			t.Fatal("expected an error for a missing rate80, got nil")
		}
	})

	t.Run("malformed itemNeeded", func(t *testing.T) {
		s := commons.NewStatSet()
		s.Set("id", "1")
		s.Set("lvl", "101")
		s.Set("exp", "1")
		s.Set("sp", "1")
		s.Set("rate76", "1")
		s.Set("rate77", "1")
		s.Set("rate78", "1")
		s.Set("rate79", "1")
		s.Set("rate80", "1")
		s.Set("itemNeeded", "not-a-pair")
		if _, err := NewEnchantSkill(s); err == nil {
			t.Fatal("expected an error for a malformed itemNeeded, got nil")
		}
	})
}

func TestFishingSkillQueries(t *testing.T) {
	trees := &Trees{Fishing: []FishingSkill{
		{ID: 1312, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1000},
		{ID: 1313, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 10},
		{ID: 1313, Level: 2, MinLevel: 4, ItemID: 57, ItemCount: 50},
		{ID: 1368, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 100, Dwarven: true},
	}}

	available := trees.FishingSkillsFor(1, false, SkillLevels{1312: 0, 1313: 0, 1368: 0})
	want := []FishingSkill{
		{ID: 1312, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1000},
		{ID: 1313, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 10},
	}
	if !equalFishingSkills(available, want) {
		t.Fatalf("FishingSkillsFor(level 1, non-dwarf) = %+v, want %+v", available, want)
	}

	if _, ok := trees.FishingSkillFor(1, false, SkillLevels{}, 1368, 1); ok {
		t.Fatal("FishingSkillFor(dwarven, non-dwarf) found a skill")
	}

	grant, ok := trees.FishingSkillFor(4, true, SkillLevels{1313: 1}, 1313, 2)
	if !ok || grant.Level != 2 || grant.ItemCount != 50 {
		t.Fatalf("FishingSkillFor(level 4, known 1313:1) = %+v, %v; want level 2", grant, ok)
	}

	if _, ok := trees.FishingSkillFor(4, true, SkillLevels{1313: 0}, 1313, 2); ok {
		t.Fatal("FishingSkillFor(skipped previous level) found a skill")
	}

	if got := trees.RequiredLevelForNextFishingSkill(1, false); got != 4 {
		t.Fatalf("RequiredLevelForNextFishingSkill(level 1, non-dwarf) = %d, want 4", got)
	}
}

func TestClanSkillQueries(t *testing.T) {
	trees := &Trees{Clan: []ClanSkill{
		{ID: 370, Level: 1, MinLevel: 5, Cost: 500, ItemID: 8166},
		{ID: 370, Level: 2, MinLevel: 5, Cost: 500, ItemID: 8166},
		{ID: 371, Level: 1, MinLevel: 6, Cost: 800, ItemID: 8169},
	}}

	available := trees.ClanSkillsFor(5, SkillLevels{})
	want := []ClanSkill{{ID: 370, Level: 1, MinLevel: 5, Cost: 500, ItemID: 8166}}
	if !equalClanSkills(available, want) {
		t.Fatalf("ClanSkillsFor(level 5, none known) = %+v, want %+v", available, want)
	}

	grant, status := trees.CheckClanSkillLearn(5, 499, SkillLevels{}, 370, 1)
	if status != LearnNeedsCost || grant.Cost != 500 {
		t.Fatalf("CheckClanSkillLearn(not enough reputation) = %+v, %v; want cost 500 and LearnNeedsCost", grant, status)
	}

	grant, status = trees.CheckClanSkillLearn(5, 500, SkillLevels{}, 370, 1)
	if status != LearnAllowed || grant.ID != 370 || grant.Level != 1 {
		t.Fatalf("CheckClanSkillLearn(enough reputation) = %+v, %v; want skill 370 level 1 and LearnAllowed", grant, status)
	}

	if _, status = trees.CheckClanSkillLearn(5, 500, SkillLevels{370: 0}, 370, 2); status != LearnUnavailable {
		t.Fatalf("CheckClanSkillLearn(skipped previous level) = %v, want LearnUnavailable", status)
	}
}

func equalFishingSkills(a, b []FishingSkill) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalClanSkills(a, b []ClanSkill) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
