package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ663MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ663MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q663", []string{"quest.Q663_SeductiveWhispers"},
		// The texts of the cards, which the reference names by constant.
		"number 66300", "number 66311", "number 66312", "number 66313", "number 66314", "number 66315", "number 66321", "number 66322", "number 66323", "number 66324", "number 66325")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ663PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ663PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q663", "quest/Q663_SeductiveWhispers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
