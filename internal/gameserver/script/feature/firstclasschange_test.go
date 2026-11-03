package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestFirstclasschangeMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestFirstclasschangeMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("firstclasschange", []string{"script.feature.FirstClassChange"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestFirstclasschangePagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestFirstclasschangePagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "firstclasschange", "feature/FirstClassChange")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
