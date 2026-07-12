package item

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestParseDropKind(t *testing.T) {
	cases := []struct {
		in   string
		want DropKind
	}{
		{"SPOIL", DropSpoil},
		{"CURRENCY", DropCurrency},
		{"DROP", DropNormal},
		{"HERB", DropHerb},
	}
	for _, c := range cases {
		got, err := ParseDropKind(c.in)
		if err != nil {
			t.Fatalf("ParseDropKind(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseDropKind(%q) = %v, want %v", c.in, got, c.want)
		}
	}

	if _, err := ParseDropKind("BOGUS"); err == nil {
		t.Fatal("ParseDropKind(\"BOGUS\") error = nil, want error")
	}
}

func TestNewDrop(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("itemid", "8600")
	set.Set("min", "1")
	set.Set("max", "3")
	set.Set("chance", "55.0")

	got, err := NewDrop(set)
	if err != nil {
		t.Fatalf("NewDrop() error: %v", err)
	}
	want := Drop{ItemID: 8600, Min: 1, Max: 3, Chance: 55.0}
	if got != want {
		t.Fatalf("NewDrop() = %+v, want %+v", got, want)
	}
}

func TestNewDropCategory(t *testing.T) {
	t.Run("explicit chance", func(t *testing.T) {
		set := commons.NewStatSet()
		set.Set("type", "HERB")
		set.Set("chance", "42.0")

		got, err := NewDropCategory(set, nil)
		if err != nil {
			t.Fatalf("NewDropCategory() error: %v", err)
		}
		if got.Kind != DropHerb || got.Chance != 42.0 {
			t.Fatalf("NewDropCategory() = %+v", got)
		}
	})

	t.Run("chance defaults to 100", func(t *testing.T) {
		set := commons.NewStatSet()
		set.Set("type", "DROP")

		got, err := NewDropCategory(set, nil)
		if err != nil {
			t.Fatalf("NewDropCategory() error: %v", err)
		}
		if got.Chance != 100.0 {
			t.Fatalf("NewDropCategory() chance = %v, want 100", got.Chance)
		}
	})

	t.Run("missing type is an error", func(t *testing.T) {
		set := commons.NewStatSet()
		if _, err := NewDropCategory(set, nil); err == nil {
			t.Fatal("NewDropCategory() error = nil, want error")
		}
	})
}

func TestDropRandomAmount(t *testing.T) {
	t.Run("fixed range", func(t *testing.T) {
		d := Drop{Min: 3, Max: 3}
		for i := 0; i < 20; i++ {
			if got := d.RandomAmount(); got != 3 {
				t.Fatalf("RandomAmount() = %d, want 3", got)
			}
		}
	})

	t.Run("within range", func(t *testing.T) {
		d := Drop{Min: 1, Max: 5}
		for i := 0; i < 200; i++ {
			got := d.RandomAmount()
			if got < 1 || got > 5 {
				t.Fatalf("RandomAmount() = %d, want in [1,5]", got)
			}
		}
	})

	t.Run("malformed range does not panic", func(t *testing.T) {
		d := Drop{Min: 5, Max: 1}
		if got := d.RandomAmount(); got != 5 {
			t.Fatalf("RandomAmount() = %d, want Min (5)", got)
		}
	})
}

func TestDropCategoryRollBoundaries(t *testing.T) {
	t.Run("zero category chance never drops", func(t *testing.T) {
		c := DropCategory{Kind: DropNormal, Chance: 0, Drops: []Drop{{ItemID: 1, Min: 1, Max: 1, Chance: 100}}}
		if got := c.Roll(1, 1); got != nil {
			t.Fatalf("Roll() = %v, want nil", got)
		}
	})

	t.Run("zero level multiplier never drops", func(t *testing.T) {
		c := DropCategory{Kind: DropNormal, Chance: 100, Drops: []Drop{{ItemID: 1, Min: 1, Max: 1, Chance: 100}}}
		if got := c.Roll(0, 1); got != nil {
			t.Fatalf("Roll() = %v, want nil", got)
		}
	})

	t.Run("zero rate never drops", func(t *testing.T) {
		c := DropCategory{Kind: DropNormal, Chance: 100, Drops: []Drop{{ItemID: 1, Min: 1, Max: 1, Chance: 100}}}
		if got := c.Roll(1, 0); got != nil {
			t.Fatalf("Roll() = %v, want nil", got)
		}
	})

	t.Run("guaranteed normal drop picks exactly one entry per rate attempt", func(t *testing.T) {
		c := DropCategory{
			Kind:   DropNormal,
			Chance: 100,
			Drops: []Drop{
				{ItemID: 10, Min: 2, Max: 2, Chance: 50},
				{ItemID: 20, Min: 3, Max: 3, Chance: 50},
			},
		}
		got := c.Roll(1, 3)
		total := int32(0)
		for id, qty := range got {
			if id != 10 && id != 20 {
				t.Fatalf("Roll() produced unexpected item %d", id)
			}
			total += qty
		}
		// 3 attempts, each contributing 2 or 3 units; both bounds are the
		// same 2/3, so the sum must land in [6, 9] and be a multiple
		// achievable by 3 picks of {2,3}.
		if total < 6 || total > 9 {
			t.Fatalf("Roll() total = %d, want in [6,9]", total)
		}
	})

	t.Run("guaranteed spoil drop evaluates every entry independently", func(t *testing.T) {
		c := DropCategory{
			Kind:   DropSpoil,
			Chance: 100,
			Drops: []Drop{
				{ItemID: 10, Min: 1, Max: 1, Chance: 100},
				{ItemID: 20, Min: 1, Max: 1, Chance: 100},
			},
		}
		got := c.Roll(1, 1)
		if got[10] != 1 || got[20] != 1 {
			t.Fatalf("Roll() = %v, want both items at 1 each", got)
		}
	})

	t.Run("fractional rate rolls one extra attempt", func(t *testing.T) {
		c := DropCategory{
			Kind:   DropSpoil,
			Chance: 100,
			Drops:  []Drop{{ItemID: 10, Min: 1, Max: 1, Chance: 100}},
		}
		got := c.Roll(1, 1.5)
		if got[10] != 2 {
			t.Fatalf("Roll() = %v, want item 10 at 2 (two attempts from a 1.5 rate)", got)
		}
	})
}

func TestRatesResolve(t *testing.T) {
	r := Rates{Spoil: 1, Currency: 2, Item: 3, ItemRaid: 4, Herb: 5}

	cases := []struct {
		kind DropKind
		raid bool
		want float64
	}{
		{DropSpoil, false, 1},
		{DropSpoil, true, 1},
		{DropCurrency, false, 2},
		{DropNormal, false, 3},
		{DropNormal, true, 4},
		{DropHerb, false, 5},
	}
	for _, c := range cases {
		if got := r.Resolve(c.kind, c.raid); got != c.want {
			t.Fatalf("Resolve(%v, %v) = %v, want %v", c.kind, c.raid, got, c.want)
		}
	}
}

func TestLevelPenaltyMultiplier(t *testing.T) {
	cases := []struct {
		name                        string
		attackerLevel, monsterLevel int32
		raid, enabled               bool
		want                        float64
	}{
		{"disabled always 1", 80, 10, false, false, 1},
		{"within monster threshold", 14, 10, false, true, 1},
		{"exactly at monster threshold", 15, 10, false, true, 1},
		{"one level past monster threshold", 16, 10, false, true, 1 - 0.18},
		{"within raid threshold", 12, 10, true, true, 1},
		{"one level past raid threshold", 13, 10, true, true, 1 - 0.18},
		{"floored at 0.1", 100, 10, false, true, 0.1},
		{"lower level attacker no penalty", 5, 10, false, true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LevelPenaltyMultiplier(c.attackerLevel, c.monsterLevel, c.raid, c.enabled)
			if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("LevelPenaltyMultiplier() = %v, want %v", got, c.want)
			}
		})
	}
}
