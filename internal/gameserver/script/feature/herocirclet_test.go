package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestHerocircletMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestHerocircletMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("herocirclet", []string{"script.feature.HeroCirclet"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestHerocircletPagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestHerocircletPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "herocirclet", "feature/HeroCirclet")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
