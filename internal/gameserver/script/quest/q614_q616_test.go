package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ614ToQ616MatchReferenceFingerprints compares each package with the
// reference class: literals, engine calls and hook shapes. The ids of the chat
// lines are named constants in the reference.
func TestQ614ToQ616MatchReferenceFingerprints(t *testing.T) {
	for _, c := range []struct {
		pkg, class string
		extra      []string
	}{
		{"q614", "quest.Q614_SlayTheEnemyCommander", nil},
		{"q615", "quest.Q615_MagicalPowerOfFire_Part1", []string{"number 61503", "number 61504"}},
		{"q616", "quest.Q616_MagicalPowerOfFire_Part2", []string{"number 61650", "number 61651"}},
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

// TestQ614ToQ616PagesAreInTheDatapack requires every page each quest names to
// be a page of its datapack directory.
func TestQ614ToQ616PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ pkg, dir string }{
		{"q614", "quest/Q614_SlayTheEnemyCommander"},
		{"q615", "quest/Q615_MagicalPowerOfFire_Part1"},
		{"q616", "quest/Q616_MagicalPowerOfFire_Part2"},
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
