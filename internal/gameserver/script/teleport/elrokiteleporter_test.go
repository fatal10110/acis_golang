package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestElrokiteleporterMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestElrokiteleporterMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("elrokiteleporter", []string{"script.teleport.ElrokiTeleporter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestElrokiteleporterPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestElrokiteleporterPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "elrokiteleporter", "teleport/ElrokiTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
