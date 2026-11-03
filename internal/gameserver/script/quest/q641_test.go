package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ641MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ641MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q641", []string{"quest.Q641_AttackSailren"},
		`string "Q126_TheNameOfEvil_2"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ641PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ641PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q641", "quest/Q641_AttackSailren")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
