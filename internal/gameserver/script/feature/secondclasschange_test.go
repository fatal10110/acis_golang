package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestSecondclasschangeMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestSecondclasschangeMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("secondclasschange", []string{"script.feature.SecondClassChange"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestSecondclasschangePagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestSecondclasschangePagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "secondclasschange", "feature/SecondClassChange")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
