package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ223MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The orc raider class id
// is a named constant in the reference.
func TestQ223MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q223", []string{"quest.Q223_TestOfTheChampion"},
		"number 45")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ223PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ223PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q223", "quest/Q223_TestOfTheChampion")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
