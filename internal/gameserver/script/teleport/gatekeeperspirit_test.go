package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestGatekeeperspiritMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestGatekeeperspiritMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("gatekeeperspirit", []string{"script.teleport.GatekeeperSpirit"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestGatekeeperspiritPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestGatekeeperspiritPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "gatekeeperspirit", "teleport/GatekeeperSpirit")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
