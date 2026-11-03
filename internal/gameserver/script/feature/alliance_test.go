package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestAllianceMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestAllianceMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("alliance", []string{"script.feature.Alliance"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestAlliancePagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestAlliancePagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "alliance", "feature/Alliance")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
