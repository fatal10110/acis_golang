package q510

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The system message id is a
// named constant in the reference.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"quest.Q510_AClansReputation"}, "number 1777")
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
	problems, err := scriptpages.CheckPackage(idx, ".", "quest/Q510_AClansReputation")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
