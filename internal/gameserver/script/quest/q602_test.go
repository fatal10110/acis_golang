package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ602MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The 4 is the index of a
// reward row's draw bound in the reference, a field of the Go reward function.
func TestQ602MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q602", []string{"quest.Q602_ShadowOfLight"}, "number 4")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ602PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ602PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q602", "quest/Q602_ShadowOfLight")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
