package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ503MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The ids of the chat lines are
// named constants in the reference.
func TestQ503MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q503", []string{"quest.Q503_PursuitOfClanAmbition"}, "number 50338", "number 50339", "number 50340", "number 50341")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ503PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ503PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q503", "quest/Q503_PursuitOfClanAmbition")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
