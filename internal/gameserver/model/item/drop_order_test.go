package item

import (
	"slices"
	"testing"
)

// TestDropMergerFollowsReferenceMapOrder pins the iteration order of a
// HashMap<Integer, Integer> built by new HashMap<>(1) and merge calls, as
// DropCategory.calculateDrop builds its result: bucket order of a table
// that starts at one bucket and resizes at the start of a merge once the
// previous one left it over 0.75 load, with a new key at its bucket's head.
func TestDropMergerFollowsReferenceMapOrder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		merges [][2]int32
		want   []rolledDrop
	}{
		{name: "none", merges: nil, want: nil},
		{name: "one key", merges: [][2]int32{{1868, 2}}, want: []rolledDrop{{1868, 2}}},
		{name: "same key sums", merges: [][2]int32{{1868, 2}, {1868, 3}}, want: []rolledDrop{{1868, 5}}},
		// Two keys: two buckets, odd ids after even ones whatever the
		// merge order.
		{name: "two buckets", merges: [][2]int32{{1869, 1}, {1868, 1}}, want: []rolledDrop{{1868, 1}, {1869, 1}}},
		// Two keys sharing bucket 0 of two: the newer key comes first.
		{name: "shared bucket newest first", merges: [][2]int32{{1868, 1}, {1870, 1}}, want: []rolledDrop{{1870, 1}, {1868, 1}}},
		// The third merge resizes to four buckets first: 1870 moves to
		// bucket 2, 1868 stays in 0 and 1872 lands at its head.
		{name: "resize splits buckets", merges: [][2]int32{{1868, 1}, {1870, 1}, {1872, 1}}, want: []rolledDrop{{1872, 1}, {1868, 1}, {1870, 1}}},
		// A merge onto a held key still resizes first: the table is at
		// four buckets when 1872 arrives, so it lands in bucket 0 apart
		// from 1870 in bucket 2.
		{name: "held-key merge resizes too", merges: [][2]int32{{1870, 1}, {1868, 1}, {1868, 1}, {1872, 1}}, want: []rolledDrop{{1872, 1}, {1868, 2}, {1870, 1}}},
		// Ids at or above 65536 spread their high half into the bucket.
		{name: "high bits spread", merges: [][2]int32{{65536, 1}, {1, 1}}, want: []rolledDrop{{1, 1}, {65536, 1}}},
	} {
		var m dropMerger
		for _, mg := range tc.merges {
			m.merge(mg[0], mg[1])
		}
		if got := m.ordered(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: ordered() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRollKillRewardKeepsCategoryRollsApart pins Monster.doItemDrop's loop
// over categories: the same item rolled by two categories stays two drops,
// and a herb category's pickups sit at the category's place among them.
func TestRollKillRewardKeepsCategoryRollsApart(t *testing.T) {
	rates := Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}
	categories := []DropCategory{
		guaranteedCategory(DropNormal, 1868, 2),
		guaranteedCategory(DropHerb, 8600, 2),
		guaranteedCategory(DropCurrency, AdenaID, 30),
		guaranteedCategory(DropNormal, 1868, 4),
	}
	want := []KillDrop{
		{ItemID: 1868, Count: 2},
		{ItemID: 8600, Count: 1, Herb: true},
		{ItemID: 8600, Count: 1, Herb: true},
		{ItemID: AdenaID, Count: 30},
		{ItemID: 1868, Count: 4},
	}
	if got := RollKillReward(categories, nil, 1, false, rates, false); !slices.Equal(got, want) {
		t.Fatalf("RollKillReward() = %v, want %v", got, want)
	}
}
