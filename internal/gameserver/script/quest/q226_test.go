package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ226MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The elven oracle's class id is
// a named constant in the reference, and the variable Gupu sets holds the
// number 1 there.
func TestQ226MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q226", []string{"quest.Q226_TestOfTheHealer"},
		"number 29", `string "1"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ226PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ226PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q226", "quest/Q226_TestOfTheHealer")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
