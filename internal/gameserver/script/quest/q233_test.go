package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ233MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The orc shaman class id is a
// named constant in the reference.
func TestQ233MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q233", []string{"quest.Q233_TestOfTheWarSpirit"},
		"number 50")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ233PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ233PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q233", "quest/Q233_TestOfTheWarSpirit")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
