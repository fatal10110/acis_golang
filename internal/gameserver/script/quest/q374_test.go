package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ374MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The port parses the reward number with a
// 32-bit size where the reference parses without a literal.
func TestQ374MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q374", []string{"quest.Q374_WhisperOfDreams_Part1"}, "number 32")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ374PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ374PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q374", "quest/Q374_WhisperOfDreams_Part1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
