package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestGrandbossteleporterMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestGrandbossteleporterMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("grandbossteleporter", []string{"script.teleport.GrandBossTeleporter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestGrandbossteleporterPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestGrandbossteleporterPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "grandbossteleporter", "teleport/GrandBossTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
