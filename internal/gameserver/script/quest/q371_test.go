package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ371MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ371MatchesReferenceFingerprints(t *testing.T) {
	// The 3 read is the capacity hint of the reference's chance table.
	problems, err := scriptfp.Check("q371", []string{"quest.Q371_ShriekOfGhosts"}, "number 3")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ371PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ371PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q371", "quest/Q371_ShriekOfGhosts")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
