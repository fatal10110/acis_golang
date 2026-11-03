package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ509MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The system message id is a named constant in the
// reference, and the reward columns its indices 2 and 3 read are struct fields
// here.
func TestQ509MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q509", []string{"quest.Q509_TheClansPrestige"}, "number 1777", "number 2", "number 3")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ509PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
// The raid pages are built from the raid number, as in the reference.
func TestQ509PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q509", "quest/Q509_TheClansPrestige", "a.htm", "b.htm")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
