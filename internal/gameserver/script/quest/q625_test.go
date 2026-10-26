package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ625MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The ids of the raid's chat lines are named
// constants in the reference.
func TestQ625MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q625", []string{"quest.Q625_TheFinestIngredients_Part2"}, "number 62503", "number 62504")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ625PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ625PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q625", "quest/Q625_TheFinestIngredients_Part2")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
