package multisell

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestPrepareCastleTax pins PreparedEntry's tax: on a list applying taxes,
// each tax ingredient's count at the castle rate is rounded half up on its
// own (Math.round) and added, with the plain adena, as one adena
// ingredient at the end; the entry keeps the tax amount for the castle. A
// list not applying taxes drops the tax ingredients whatever the rate.
func TestPrepareCastleTax(t *testing.T) {
	const gem int32 = 1888
	for _, tt := range []struct {
		name       string
		apply      bool
		rate       float64
		taxCounts  []int
		wantTax    int
		wantAdena  int // 0 for no adena ingredient
		plainAdena int
	}{
		{"rate 15", true, 0.15, []int{1000}, 150, 650, 500},
		{"half rounds up", true, 0.25, []int{10}, 3, 503, 500},    // 2.5 -> 3
		{"one and a half", true, 0.25, []int{6}, 2, 502, 500},     // 1.5 -> 2
		{"under half", true, 0.15, []int{3}, 0, 500, 500},         // 0.45 -> 0
		{"rounded each", true, 0.25, []int{1, 1}, 0, 500, 500},    // 0.25 + 0.25, not 0.5
		{"rounded each up", true, 0.25, []int{3, 3}, 2, 502, 500}, // 0.75 + 0.75
		{"tax only", true, 0.05, []int{30}, 2, 2, 0},              // 1.5 -> 2
		{"no owner", true, 0, []int{1000}, 0, 500, 500},
		{"list without taxes", false, 0.15, []int{1000}, 0, 500, 500},
		{"nothing at all", false, 0.15, []int{1000}, 0, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ingredients := []Ingredient{}
			for _, c := range tt.taxCounts {
				ingredients = append(ingredients, Ingredient{ItemID: item.AdenaID, Count: c, TaxIngredient: true})
			}
			ingredients = append(ingredients, Ingredient{ItemID: gem, Count: 2})
			if tt.plainAdena > 0 {
				ingredients = append(ingredients, Ingredient{ItemID: item.AdenaID, Count: tt.plainAdena})
			}
			list := &List{ID: 1, ApplyTaxes: tt.apply, Entries: []Entry{NewEntry(ingredients, []Ingredient{{ItemID: gem, Count: 1}})}}

			for _, prepared := range []*List{list.Prepare(tt.rate), list.PrepareFor([]Held{{ItemID: gem}}, tt.rate)} {
				if len(prepared.Entries) != 1 {
					t.Fatalf("entries = %d, want 1", len(prepared.Entries))
				}
				e := prepared.Entries[0]
				if got := e.TaxAmount(); got != tt.wantTax {
					t.Fatalf("TaxAmount = %d, want %d", got, tt.wantTax)
				}
				want := []Ingredient{{ItemID: gem, Count: 2}}
				if tt.wantAdena > 0 {
					want = append(want, Ingredient{ItemID: item.AdenaID, Count: tt.wantAdena})
				}
				if !slices.Equal(e.Ingredients, want) {
					t.Fatalf("ingredients = %+v, want %+v", e.Ingredients, want)
				}
			}
		})
	}
}
