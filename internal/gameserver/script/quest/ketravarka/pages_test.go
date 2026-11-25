package ketravarka

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
		{"q605.go", "quest/Q605_AllianceWithKetraOrcs"},
		{"q606.go", "quest/Q606_WarWithVarkaSilenos"},
		{"q607.go", "quest/Q607_ProveYourCourage"},
		{"q608.go", "quest/Q608_SlayTheEnemyCommander"},
		{"q609.go", "quest/Q609_MagicalPowerOfWater_Part1"},
		{"q611.go", "quest/Q611_AllianceWithVarkaSilenos"},
		{"q612.go", "quest/Q612_WarWithKetraOrcs"},
		{"q613.go", "quest/Q613_ProveYourCourage"},
		{"q614.go", "quest/Q614_SlayTheEnemyCommander"},
		{"q615.go", "quest/Q615_MagicalPowerOfFire_Part1"},
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
