package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ603MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ603MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q603", []string{"quest.Q603_DaimonTheWhiteEyed_Part1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ603PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ603PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q603", "quest/Q603_DaimonTheWhiteEyed_Part1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
