package manor

import (
	"math"
	"slices"
)

// SeedMaxPrice is the highest price a castle may sell the seed at: ten
// times its reference price.
func (s Seed) SeedMaxPrice() int32 { return s.SeedReferencePrice * 10 }

// SeedMinPrice is the lowest price a castle may sell the seed at: 60% of
// its reference price, truncated.
func (s Seed) SeedMinPrice() int32 { return truncate(float64(s.SeedReferencePrice) * 0.6) }

// CropMaxPrice is the highest price a castle may buy the crop at: ten
// times its reference price.
func (s Seed) CropMaxPrice() int32 { return s.CropReferencePrice * 10 }

// CropMinPrice is the lowest price a castle may buy the crop at: 60% of
// its reference price, truncated.
func (s Seed) CropMinPrice() int32 { return truncate(float64(s.CropReferencePrice) * 0.6) }

// truncate narrows v toward zero to an int32, saturating at its range.
func truncate(v float64) int32 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= math.MaxInt32:
		return math.MaxInt32
	case v <= math.MinInt32:
		return math.MinInt32
	}
	return int32(v)
}

// ApplyReferencePrices sets each seed's seed and crop reference prices to
// the reference price price reports for the item, 1 for an item it does
// not know. An id outside the int32 range names no item and also gets 1.
func (t *Table) ApplyReferencePrices(price func(itemID int32) (int32, bool)) {
	ref := func(id int) int32 {
		if id < math.MinInt32 || id > math.MaxInt32 {
			return 1
		}
		if p, ok := price(int32(id)); ok {
			return p
		}
		return 1
	}
	set := func(s *Seed) {
		s.SeedReferencePrice, s.CropReferencePrice = ref(s.SeedID), ref(s.CropID)
	}
	for i := range t.Manors {
		seeds := slices.Clone(t.Manors[i].Seeds)
		for j := range seeds {
			set(&seeds[j])
		}
		t.Manors[i].Seeds = seeds
	}
	for id, s := range t.SeedsByID {
		set(&s)
		t.SeedsByID[id] = s
	}
}

// Seeds returns every seed, one per seed id, in the table's iteration
// order: by seed id modulo the smallest power-of-two table, of at least
// 16 slots, that holds them at three quarters full, and by first load
// within one slot.
func (t *Table) Seeds() []Seed {
	if t == nil {
		return nil
	}
	out := make([]Seed, 0, len(t.order))
	for _, id := range t.order {
		out = append(out, t.SeedsByID[id])
	}
	return out
}

// SeedByCrop returns the first seed, in Seeds order, growing cropID.
func (t *Table) SeedByCrop(cropID int) (Seed, bool) {
	for _, s := range t.Seeds() {
		if s.CropID == cropID {
			return s, true
		}
	}
	return Seed{}, false
}

// SeedByCropForCastle returns the first seed, in Seeds order, of castle
// castleID's manor growing cropID.
func (t *Table) SeedByCropForCastle(cropID, castleID int) (Seed, bool) {
	for _, s := range t.Seeds() {
		if s.CropID == cropID && s.CastleID == castleID {
			return s, true
		}
	}
	return Seed{}, false
}

// SeedsForCastle returns the seeds of castle castleID's manor, in Seeds
// order.
func (t *Table) SeedsForCastle(castleID int) []Seed {
	var out []Seed
	for _, s := range t.Seeds() {
		if s.CastleID == castleID {
			out = append(out, s)
		}
	}
	return out
}

// Crops returns the first seed, in Seeds order, of each crop.
func (t *Table) Crops() []Seed {
	var out []Seed
	seen := map[int]bool{}
	for _, s := range t.Seeds() {
		if !seen[s.CropID] {
			seen[s.CropID] = true
			out = append(out, s)
		}
	}
	return out
}

// CropIDs returns every crop id once, ordered as Seeds orders seed ids:
// by id modulo the crop table's size, then by first appearance in Seeds.
func (t *Table) CropIDs() []int {
	var ids []int
	seen := map[int]bool{}
	for _, s := range t.Seeds() {
		if !seen[s.CropID] {
			seen[s.CropID] = true
			ids = append(ids, s.CropID)
		}
	}
	return hashOrder(ids)
}

// SeedReward returns reward item 1 of the first seed growing cropID when
// rewardType is 1, its reward item 2 otherwise, and 0 when no seed grows
// cropID.
func (t *Table) SeedReward(cropID, rewardType int) int {
	s, ok := t.SeedByCrop(cropID)
	switch {
	case !ok:
		return 0
	case rewardType == 1:
		return s.Reward1
	}
	return s.Reward2
}

// hashOrder orders distinct int keys, given in insertion order, as a
// hash table of power-of-two size iterates them: by slot, the key's hash
// modulo the table size, then by insertion within a slot. The table starts
// at 16 slots and doubles whenever it is more than three quarters full.
func hashOrder(keys []int) []int {
	size := 16
	for len(keys) > size*3/4 {
		size *= 2
	}
	slot := func(k int) int {
		h := uint32(int32(k))
		return int((h ^ h>>16) & uint32(size-1))
	}
	out := slices.Clone(keys)
	slices.SortStableFunc(out, func(a, b int) int { return slot(a) - slot(b) })
	return out
}
