package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestHuntinggroundsteleporterMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestHuntinggroundsteleporterMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("huntinggroundsteleporter", []string{"script.teleport.HuntingGroundsTeleporter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestHuntinggroundsteleporterPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestHuntinggroundsteleporterPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "huntinggroundsteleporter", "teleport/HuntingGroundsTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
