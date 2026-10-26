package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ378MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference sizes its reward map
// with a hint the port does not need.
func TestQ378MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q378", []string{"quest.Q378_MagnificentFeast"}, "number 9")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ378PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ378PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q378", "quest/Q378_MagnificentFeast")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
