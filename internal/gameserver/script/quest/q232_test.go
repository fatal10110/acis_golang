package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ232MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference names the Orc
// Shaman by its class, the Go package by number.
func TestQ232MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q232", []string{"quest.Q232_TestOfTheLord"}, "number 50")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ232PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ232PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q232", "quest/Q232_TestOfTheLord")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
