package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ634MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ634MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q634", []string{"quest.Q634_InSearchOfFragmentsOfDimension"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ634PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ634PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q634", "quest/Q634_InSearchOfFragmentsOfDimension")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
