package main

import (
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestLoadManorAppliesReferencePrices loads the shipped manors.xml with the
// shipped items: each seed carries the reference prices of its seed and
// crop items (data/xml/items 5000-5099.xml and 7000-7099.xml: Seed: Dark
// Coda 5016 is priced 5, Dark Coda 5073 50, Seed: Red Cobol 7051 20), and
// the price bounds follow from them.
func TestLoadManorAppliesReferencePrices(t *testing.T) {
	t.Parallel()
	root := datapack.Path(t, "data", "xml")
	items, err := gamexml.LoadItemTemplates(filepath.Join(root, "items"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	seeds, _, err := loadManor(root, items, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		seedID                                      int32
		seedRef, seedMin, seedMax, cropRef, cropMin int32
	}{
		{5016, 5, 3, 50, 50, 30},
		{7051, 20, 12, 200, 0, 0},
	} {
		s, ok := seeds.Seed(tc.seedID)
		if !ok {
			t.Fatalf("seed %d not loaded", tc.seedID)
		}
		if s.SeedReferencePrice != tc.seedRef || s.SeedMinPrice() != tc.seedMin || s.SeedMaxPrice() != tc.seedMax {
			t.Errorf("seed %d seed prices = %d/%d/%d, want %d/%d/%d", tc.seedID,
				s.SeedReferencePrice, s.SeedMinPrice(), s.SeedMaxPrice(), tc.seedRef, tc.seedMin, tc.seedMax)
		}
		if tc.cropRef != 0 && (s.CropReferencePrice != tc.cropRef || s.CropMinPrice() != tc.cropMin) {
			t.Errorf("seed %d crop prices = %d/%d, want %d/%d", tc.seedID, s.CropReferencePrice, s.CropMinPrice(), tc.cropRef, tc.cropMin)
		}
	}
	for _, s := range seeds.Seeds() {
		if s.SeedReferencePrice <= 0 || s.CropReferencePrice <= 0 {
			t.Fatalf("seed %d has no reference price: %+v", s.SeedID, s)
		}
	}
}
