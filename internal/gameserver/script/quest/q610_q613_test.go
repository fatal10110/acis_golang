package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ61xMatchesReferenceFingerprints compares each package with the
// reference class: literals, engine calls and hook shapes. The ids of
// Ashutar's two chat lines are named constants in the reference, and the
// 15 and 21 of Q611 are the initial sizes of its two chance maps, which Go
// has no use for.
func TestQ61xMatchesReferenceFingerprints(t *testing.T) {
	for _, c := range []struct {
		pkg, class string
		extra      []string
	}{
		{"q610", "quest.Q610_MagicalPowerOfWater_Part2", []string{"number 61050", "number 61051"}},
		{"q611", "quest.Q611_AllianceWithVarkaSilenos", []string{"number 15", "number 21"}},
		{"q612", "quest.Q612_WarWithKetraOrcs", nil},
		{"q613", "quest.Q613_ProveYourCourage", nil},
	} {
		t.Run(c.pkg, func(t *testing.T) {
			problems, err := scriptfp.Check(c.pkg, []string{c.class}, c.extra...)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range problems {
				t.Error(p)
			}
		})
	}
}

// TestQ61xPagesAreInTheDatapack requires every page each quest names to be a
// page of its datapack directory.
func TestQ61xPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ pkg, dir string }{
		{"q610", "quest/Q610_MagicalPowerOfWater_Part2"},
		{"q611", "quest/Q611_AllianceWithVarkaSilenos"},
		{"q612", "quest/Q612_WarWithKetraOrcs"},
		{"q613", "quest/Q613_ProveYourCourage"},
	} {
		t.Run(c.pkg, func(t *testing.T) {
			problems, err := scriptpages.CheckPackage(idx, c.pkg, c.dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range problems {
				t.Error(p)
			}
		})
	}
}
