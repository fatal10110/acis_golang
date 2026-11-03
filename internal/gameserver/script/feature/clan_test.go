package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestClanMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestClanMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("clan", []string{"script.feature.Clan"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestClanPagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestClanPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "clan", "feature/Clan")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
