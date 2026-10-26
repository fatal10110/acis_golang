package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ419MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference sizes its drop
// map with 14, a capacity hint the Go switch does not need.
func TestQ419MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q419", []string{"quest.Q419_GetAPet"}, "number 14")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ419PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ419PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q419", "quest/Q419_GetAPet")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
