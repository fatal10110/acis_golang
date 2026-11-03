package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ218MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads the
// class tier through the class itself; ClassLevel reads both.
func TestQ218MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q218", []string{"quest.Q218_TestimonyOfLife"},
		"call Player.getClassId")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ218PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ218PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q218", "quest/Q218_TestimonyOfLife")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
