package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ011MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ011MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q011", []string{"quest.Q011_SecretMeetingWithKetraOrcs"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ011PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ011PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q011", "quest/Q011_SecretMeetingWithKetraOrcs")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
