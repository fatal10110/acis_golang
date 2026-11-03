package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ225MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the test
// is open to, past the rogue's 7, are named constants in the reference.
func TestQ225MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q225", []string{"quest.Q225_TestOfTheSearcher"},
		"number 22", "number 35", "number 54")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ225PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ225PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q225", "quest/Q225_TestOfTheSearcher")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
