package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ620MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The chat line is a named
// constant in the reference, and 20 and 79 are the sizes of its tables.
func TestQ620MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q620", []string{"quest.Q620_FourGoblets"}, "number 1000502", "number 20", "number 79", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ620PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ620PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q620", "quest/Q620_FourGoblets")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
