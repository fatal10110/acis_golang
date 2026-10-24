package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ012MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ012MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q012", []string{"quest.Q012_SecretMeetingWithVarkaSilenos"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ012PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ012PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q012", "quest/Q012_SecretMeetingWithVarkaSilenos")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
