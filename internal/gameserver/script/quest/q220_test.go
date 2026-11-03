package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ220MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads the
// class tier through the class itself; ClassLevel reads both. The reference
// names its five chat lines by constant (ID_22051 and so on), the Go package
// by number.
func TestQ220MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q220", []string{"quest.Q220_TestimonyOfGlory"},
		"call Player.getClassId", "number 22051", "number 22052", "number 22055", "number 22056", "number 22057")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ220PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ220PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q220", "quest/Q220_TestimonyOfGlory")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
