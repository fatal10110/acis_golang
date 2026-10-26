package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ227MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference names its two
// chat lines by constant (ID_22719, ID_22720) and the Shillien oracle by its
// class, the Go package by number.
func TestQ227MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q227", []string{"quest.Q227_TestOfTheReformer"},
		"number 22719", "number 22720", "number 42")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ227PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ227PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q227", "quest/Q227_TestOfTheReformer")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
