package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ306MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads its
// drop rows by position; the port names the fields, so the third field's index
// is not one of its literals.
func TestQ306MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q306", []string{"quest.Q306_CrystalsOfFireAndIce"}, "number 2")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ306PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ306PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q306", "quest/Q306_CrystalsOfFireAndIce")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
