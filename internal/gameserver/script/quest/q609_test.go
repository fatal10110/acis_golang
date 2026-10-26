package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ609MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The ids of the two chat lines
// are named constants in the reference.
func TestQ609MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q609", []string{"quest.Q609_MagicalPowerOfWater_Part1"}, "number 60903", "number 60904")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ609PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ609PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q609", "quest/Q609_MagicalPowerOfWater_Part1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
