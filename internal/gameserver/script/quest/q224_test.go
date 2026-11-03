package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ224MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the test
// is open to, past the rogue's 7, are named constants in the reference.
func TestQ224MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q224", []string{"quest.Q224_TestOfSagittarius"},
		"number 22", "number 35")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ224PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ224PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q224", "quest/Q224_TestOfSagittarius")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
