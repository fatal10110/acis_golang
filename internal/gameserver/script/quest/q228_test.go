package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ228MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The wizards' class ids and
// the singing flowers' NpcString ids are named constants in the reference.
func TestQ228MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q228", []string{"quest.Q228_TestOfMagus"},
		"number 11", "number 26", "number 22819", "number 22820", "number 22821")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ228PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ228PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q228", "quest/Q228_TestOfMagus")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
