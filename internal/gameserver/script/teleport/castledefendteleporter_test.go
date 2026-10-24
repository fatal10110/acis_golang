package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestCastledefendteleporterMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestCastledefendteleporterMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("castledefendteleporter", []string{"script.teleport.CastleDefendTeleporter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestCastledefendteleporterPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestCastledefendteleporterPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "castledefendteleporter", "teleport/CastleDefendTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
