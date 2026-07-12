package item

import "testing"

func guaranteedCategory(kind DropKind, itemID int32, amount int32) DropCategory {
	return DropCategory{
		Kind:   kind,
		Chance: 100,
		Drops:  []Drop{{ItemID: itemID, Min: amount, Max: amount, Chance: 100}},
	}
}

func TestRollKillRewardSpoilRequiresMarkedPool(t *testing.T) {
	rates := Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}
	categories := []DropCategory{guaranteedCategory(DropSpoil, 999, 1)}

	t.Run("nil pool skips spoil category", func(t *testing.T) {
		items, herbs := RollKillReward(categories, nil, 1, false, rates, false)
		if items != nil || herbs != nil {
			t.Fatalf("RollKillReward() = (%v, %v), want (nil, nil)", items, herbs)
		}
	})

	t.Run("unmarked pool skips spoil category", func(t *testing.T) {
		var pool SpoilPool
		items, herbs := RollKillReward(categories, &pool, 1, false, rates, false)
		if items != nil || herbs != nil {
			t.Fatalf("RollKillReward() = (%v, %v), want (nil, nil)", items, herbs)
		}
		if pool.Sweepable() {
			t.Fatal("pool became sweepable despite being unmarked")
		}
	})

	t.Run("marked pool collects the spoil roll", func(t *testing.T) {
		var pool SpoilPool
		pool.Mark(1)
		items, herbs := RollKillReward(categories, &pool, 1, false, rates, false)
		if items != nil || herbs != nil {
			t.Fatalf("RollKillReward() = (%v, %v), want (nil, nil) — spoil goes to the pool", items, herbs)
		}
		got := pool.Sweep()
		if got[999] != 1 {
			t.Fatalf("pool.Sweep() = %v, want {999: 1}", got)
		}
	})
}

func TestRollKillRewardHerbSplitsIntoPickups(t *testing.T) {
	rates := Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}
	categories := []DropCategory{guaranteedCategory(DropHerb, 500, 3)}

	t.Run("auto loot collapses to one pickup", func(t *testing.T) {
		items, herbs := RollKillReward(categories, nil, 1, false, rates, true)
		if items != nil {
			t.Fatalf("items = %v, want nil", items)
		}
		want := []HerbPickup{{ItemID: 500, Amount: 1, AutoLoot: true}}
		if len(herbs) != 1 || herbs[0] != want[0] {
			t.Fatalf("herbs = %v, want %v", herbs, want)
		}
	})

	t.Run("manual pickup yields one per rolled unit", func(t *testing.T) {
		_, herbs := RollKillReward(categories, nil, 1, false, rates, false)
		if len(herbs) != 3 {
			t.Fatalf("herbs = %v, want 3 entries", herbs)
		}
	})
}

func TestRollKillRewardMergesNormalAndCurrencyIntoItems(t *testing.T) {
	rates := Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}
	categories := []DropCategory{
		guaranteedCategory(DropCurrency, 57, 10),
		guaranteedCategory(DropNormal, 1000, 2),
	}

	items, herbs := RollKillReward(categories, nil, 1, false, rates, false)
	if herbs != nil {
		t.Fatalf("herbs = %v, want nil", herbs)
	}
	want := map[int32]int32{57: 10, 1000: 2}
	if len(items) != len(want) || items[57] != want[57] || items[1000] != want[1000] {
		t.Fatalf("items = %v, want %v", items, want)
	}
}

func TestRollKillRewardUsesRaidRateForNormalDrops(t *testing.T) {
	// A rate below 1 means the category never rolls (Roll's loop condition
	// is float64(i) < rate, so rate 0 never enters the loop). Using Item=0,
	// ItemRaid=1 proves which rate a raid kill actually resolves to.
	rates := Rates{Item: 0, ItemRaid: 1}
	categories := []DropCategory{guaranteedCategory(DropNormal, 1000, 5)}

	items, _ := RollKillReward(categories, nil, 1, false, rates, false)
	if items != nil {
		t.Fatalf("non-raid items = %v, want nil (Item rate is 0)", items)
	}

	items, _ = RollKillReward(categories, nil, 1, true, rates, false)
	if items[1000] != 5 {
		t.Fatalf("raid items = %v, want {1000: 5} (ItemRaid rate is 1)", items)
	}
}
