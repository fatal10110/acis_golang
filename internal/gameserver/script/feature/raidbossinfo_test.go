package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestRaidbossinfoMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The base and bit size of
// the event's int32 parse are the Go call's own.
func TestRaidbossinfoMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("raidbossinfo", []string{"script.feature.RaidbossInfo"},
		"number 10", "number 32")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestRaidbossinfoPagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestRaidbossinfoPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "raidbossinfo", "feature/RaidbossInfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
