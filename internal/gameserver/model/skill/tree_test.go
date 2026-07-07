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
