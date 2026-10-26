package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ628MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The 8 is the size the reference
// gives its map of blessings.
func TestQ628MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q628", []string{"quest.Q628_HuntOfTheGoldenRamMercenaryForce"}, "number 8")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ628PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ628PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q628", "quest/Q628_HuntOfTheGoldenRamMercenaryForce")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
