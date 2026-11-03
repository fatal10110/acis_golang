package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ229MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference names its four
// chat lines by constant (ID_22933 to ID_22936) and the Human Wizard and the Palus Knight by
// their class, the Go package by number; the memo gem456 is set by integer there and
// by decimal string here.
func TestQ229MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q229", []string{"quest.Q229_TestOfWitchcraft"},
		"number 22933", "number 22934", "number 22935", "number 22936", "number 11", "number 32",
		`string "1"`, `string "2"`, `string "3"`, `string "4"`, `string "5"`, `string "6"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ229PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ229PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q229", "quest/Q229_TestOfWitchcraft")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
