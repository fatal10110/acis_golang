package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestPaganteleporterMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestPaganteleporterMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("paganteleporter", []string{"script.teleport.PaganTeleporter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPaganteleporterPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestPaganteleporterPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "paganteleporter", "teleport/PaganTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
