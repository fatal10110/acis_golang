package raidaltar

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestPagesAreInTheDatapack requires every page each quest names to be a page
// of its own datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ file, dir string }{
		{"q604.go", "quest/Q604_DaimonTheWhiteEyed_Part2"},
		{"q610.go", "quest/Q610_MagicalPowerOfWater_Part2"},
		{"q616.go", "quest/Q616_MagicalPowerOfFire_Part2"},
		{"q625.go", "quest/Q625_TheFinestIngredients_Part2"},
	} {
		t.Run(c.file, func(t *testing.T) {
			problems, err := scriptpages.CheckFiles(idx, ".", c.dir, []string{c.file})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range problems {
				t.Error(p)
			}
		})
	}
}
