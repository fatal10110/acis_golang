package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ373MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ373MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q373", []string{"quest.Q373_SupplierOfReagents"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ373PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ373PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q373", "quest/Q373_SupplierOfReagents")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
