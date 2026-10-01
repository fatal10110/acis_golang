package exchange

import (
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
)

// TestMergeIngredients pins the merge ahead of the ingredient check:
// ingredients sharing an item id and enchant level add up at the first one's
// place, which then reads at enchant level 0, and a sum past the largest
// int32 is refused. The ingredient check behind it would refuse such a sum
// too, since a Go int does not wrap there; this keeps the refusal where the
// merge happens.
func TestMergeIngredients(t *testing.T) {
	t.Parallel()
	got, ok := mergeIngredients([]multisell.Ingredient{
		{ItemID: 30, Count: 1, EnchantLevel: 4},
		{ItemID: 57, Count: 10},
		{ItemID: 30, Count: 2, EnchantLevel: 4},
		{ItemID: 30, Count: 1, EnchantLevel: 5},
	})
	want := []multisell.Ingredient{
		{ItemID: 30, Count: 3},
		{ItemID: 57, Count: 10},
		{ItemID: 30, Count: 1, EnchantLevel: 5},
	}
	if !ok || !slices.Equal(got, want) {
		t.Fatalf("merge = %+v, %v; want %+v", got, ok, want)
	}

	if _, ok := mergeIngredients([]multisell.Ingredient{
		{ItemID: 9102, Count: math.MaxInt32 - 1},
		{ItemID: 9102, Count: 1},
	}); !ok {
		t.Fatal("a sum of exactly the largest int32 was refused")
	}
	if got, ok := mergeIngredients([]multisell.Ingredient{
		{ItemID: 9102, Count: math.MaxInt32},
		{ItemID: 9102, Count: 1},
	}); ok {
		t.Fatalf("a sum past the largest int32 merged to %+v", got)
	}
}
