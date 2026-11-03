package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ636MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
//
// Exempt: the reference's zone-enter hook ends with a call to the base
// quest's, which does nothing, so the Go hook makes no parent call.
func TestQ636MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q636", []string{"quest.Q636_TruthBeyondTheGate"},
		"hook onZoneEnter direct same", "hook onZoneEnter none")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ636PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ636PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q636", "quest/Q636_TruthBeyondTheGate")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
