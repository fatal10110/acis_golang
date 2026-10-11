package newbiehelper

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

const path = "script.feature.NewbieHelper"

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference names the
// starting classes by enum and the blessing by its frequent-skill entry;
// the port writes their ids. 32 is the bit size of the radar id parse,
// the range Integer.parseInt accepts.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{path},
		"number 18", "number 31", "number 38", "number 44", "number 49", "number 53", "number 5182", "number 32")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory. The advice page suffixes are fragments
// appended to a guide's page stem.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "feature/NewbieHelper",
		"002.htm", "_f05.htm", "_f10.htm", "_f15.htm", "_f20.htm", "_m07.htm", "_m14.htm", "_m20.htm")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
