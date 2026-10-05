package manor

import (
	"slices"
	"testing"
)

// Reference: Seed (constructor reference prices, get*MinPrice and
// get*MaxPrice) and CastleManorManager (the _seeds HashMap and its
// lookups). Expected values are worked by hand from those methods.

// TestApplyReferencePrices sets each seed's seed and crop reference
// prices from the item table, 1 for an item it does not know, and derives
// the bounds: ten times the price, and 60% of it truncated.
func TestApplyReferencePrices(t *testing.T) {
	t.Parallel()
	table := NewTable([]Manor{{ID: 1, Seeds: []Seed{
		{CropID: 5073, SeedID: 5016, CastleID: 1},
		{CropID: 5074, SeedID: 5017, CastleID: 1},
	}}})
	prices := map[int32]int32{5016: 5, 5073: 50, 5017: 7}
	table.ApplyReferencePrices(func(id int32) (int32, bool) {
		p, ok := prices[id]
		return p, ok
	})
	for _, tc := range []struct {
		seedID                                               int32
		seedRef, seedMin, seedMax, cropRef, cropMin, cropMax int32
	}{
		{5016, 5, 3, 50, 50, 30, 500},
		// 7 * 0.6 = 4.2 truncates to 4; crop 5074 is no known item: 1,
		// whose 0.6 truncates to 0.
		{5017, 7, 4, 70, 1, 0, 10},
	} {
		s, ok := table.Seed(tc.seedID)
		if !ok {
			t.Fatalf("seed %d missing", tc.seedID)
		}
		got := []int32{s.SeedReferencePrice, s.SeedMinPrice(), s.SeedMaxPrice(), s.CropReferencePrice, s.CropMinPrice(), s.CropMaxPrice()}
		want := []int32{tc.seedRef, tc.seedMin, tc.seedMax, tc.cropRef, tc.cropMin, tc.cropMax}
		if !slices.Equal(got, want) {
			t.Errorf("seed %d prices = %v, want %v", tc.seedID, got, want)
		}
		if m := table.Manors[0].Seeds; !slices.Contains(m, s) {
			t.Errorf("seed %d in the manor list does not carry its prices", tc.seedID)
		}
	}
}

// TestSeedOrder pins the iteration order of the seed lookups: by seed id
// modulo the table size (16 slots up to 12 seeds, 32 up to 24, ...), then
// by first load within a slot. A repeated seed id keeps the place of its
// first row and the values of its last.
func TestSeedOrder(t *testing.T) {
	t.Parallel()
	table := NewTable([]Manor{
		{ID: 1, Seeds: []Seed{
			{SeedID: 100, CropID: 7, CastleID: 1},            // slot 4
			{SeedID: 33, CropID: 8, CastleID: 1},             // slot 1
			{SeedID: 17, CropID: 7, CastleID: 1},             // slot 1, after 33
			{SeedID: 20, CropID: 9, CastleID: 1, Reward1: 1}, // slot 4, after 100
		}},
		{ID: 2, Seeds: []Seed{
			{SeedID: 2, CropID: 9, CastleID: 2, Reward1: 4, Reward2: 5}, // slot 2
			{SeedID: 20, CropID: 9, CastleID: 2, Reward1: 2},            // repeats 20
		}},
	})
	var got []int
	for _, s := range table.Seeds() {
		got = append(got, s.SeedID)
	}
	if want := []int{33, 17, 2, 100, 20}; !slices.Equal(got, want) {
		t.Fatalf("Seeds order = %v, want %v", got, want)
	}
	if s, _ := table.Seed(20); s.CastleID != 2 || s.Reward1 != 2 {
		t.Fatalf("repeated seed 20 = %+v, want its last row", s)
	}
	if s, ok := table.SeedByCrop(9); !ok || s.SeedID != 2 {
		t.Fatalf("SeedByCrop(9) = %+v, want seed 2, the first in order", s)
	}
	if s, ok := table.SeedByCropForCastle(9, 2); !ok || s.SeedID != 2 {
		t.Fatalf("SeedByCropForCastle(9, 2) = %+v, want seed 2", s)
	}
	if _, ok := table.SeedByCropForCastle(8, 2); ok {
		t.Fatal("SeedByCropForCastle(8, 2) found a seed of another castle")
	}
	var crops []int
	for _, s := range table.Crops() {
		crops = append(crops, s.SeedID)
	}
	if want := []int{33, 17, 2}; !slices.Equal(crops, want) {
		t.Fatalf("Crops seeds = %v, want the first seed of each crop %v", crops, want)
	}
	// Crop ids 8, 7, 9 in first appearance; slots 8, 7, 9.
	if got, want := table.CropIDs(), []int{7, 8, 9}; !slices.Equal(got, want) {
		t.Fatalf("CropIDs = %v, want %v", got, want)
	}
	var castle1 []int
	for _, s := range table.SeedsForCastle(1) {
		castle1 = append(castle1, s.SeedID)
	}
	if want := []int{33, 17, 100}; !slices.Equal(castle1, want) {
		t.Fatalf("SeedsForCastle(1) = %v, want %v", castle1, want)
	}
	if got := table.SeedReward(9, 1); got != 4 {
		t.Fatalf("SeedReward(9, 1) = %d, want 4", got)
	}
	if got := table.SeedReward(9, 2); got != 5 {
		t.Fatalf("SeedReward(9, 2) = %d, want 5", got)
	}
	if got := table.SeedReward(99, 1); got != 0 {
		t.Fatalf("SeedReward(99, 1) = %d, want 0", got)
	}
}

// TestHashOrderGrowsTheTable pins the table size: 13 keys need 32 slots,
// so 16 and 48 no longer share a slot with 0 and 32.
func TestHashOrderGrowsTheTable(t *testing.T) {
	t.Parallel()
	keys := []int{48, 32, 16, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	if got, want := hashOrder(keys[:12]), []int{48, 32, 16, 0, 1, 2, 3, 4, 5, 6, 7, 8}; !slices.Equal(got, want) {
		t.Fatalf("hashOrder of 12 keys = %v, want %v", got, want)
	}
	if got, want := hashOrder(keys), []int{32, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 48, 16}; !slices.Equal(got, want) {
		t.Fatalf("hashOrder of 13 keys = %v, want %v", got, want)
	}
}
