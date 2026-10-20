package q218

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads the
// class tier through the class itself; ClassLevel reads both.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"quest.Q218_TestimonyOfLife"},
		"call Player.getClassId")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "quest/Q218_TestimonyOfLife")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
