package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestNoblesseteleporterMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestNoblesseteleporterMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("noblesseteleporter", []string{"script.teleport.NoblesseTeleporter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestNoblesseteleporterPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestNoblesseteleporterPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "noblesseteleporter", "teleport/NoblesseTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
