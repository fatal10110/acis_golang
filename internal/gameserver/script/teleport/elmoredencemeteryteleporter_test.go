package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestElmoredencemeteryteleporterMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestElmoredencemeteryteleporterMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("elmoredencemeteryteleporter", []string{"script.teleport.ElmoredenCemeteryTeleporter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestElmoredencemeteryteleporterPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestElmoredencemeteryteleporterPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "elmoredencemeteryteleporter", "teleport/ElmoredenCemeteryTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
