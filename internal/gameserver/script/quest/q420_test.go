package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ420MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// monsters' lines are named constants in the reference, and the quest
// variables are set to numbers it writes as strings.
func TestQ420MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q420", []string{"quest.Q420_LittleWing"},
		"number 42046", "number 42047", "number 42048", "number 42049", `string "1"`, `string "2"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ420PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ420PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q420", "quest/Q420_LittleWing")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
