package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ417MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ417MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q417", []string{"quest.Q417_PathToBecomeAScavenger"}, "number 53", "number 54")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ417PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ417PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q417", "quest/Q417_PathToBecomeAScavenger")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
