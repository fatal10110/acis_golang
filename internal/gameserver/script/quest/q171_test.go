package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ171MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString id of the
// support troop's line is a named constant in the reference.
func TestQ171MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q171", []string{"quest.Q171_ActsOfEvil"}, "number 17151")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ171PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ171PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q171", "quest/Q171_ActsOfEvil")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
