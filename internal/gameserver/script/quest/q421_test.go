package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ421MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
//
// Exempt: the flute's object id is read from the held-item list as a field,
// so no object id call is made; the chat lines are named constants in the
// reference.
func TestQ421MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q421", []string{"quest.Q421_LittleWingsBigAdventure"},
		"call WorldObject.getObjectId",
		"number 42111", "number 42112", "number 42113", "number 42114", "number 42115", "number 42116", "number 42117", "number 42118", "number 42119", "number 42120")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ421PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ421PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q421", "quest/Q421_LittleWingsBigAdventure")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
