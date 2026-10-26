package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ348MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// NPCs' lines are named constants in the reference.
func TestQ348MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q348", []string{"quest.Q348_AnArrogantSearch"},
		"number 34830", "number 34831", "number 34832", "number 34833", "number 34835", "number 34836", "number 34837", "number 34838", "number 34839")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ348PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ348PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q348", "quest/Q348_AnArrogantSearch")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
