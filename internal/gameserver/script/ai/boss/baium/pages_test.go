package baium

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestVortexPagesAreInTheDatapack requires every page the package names, the
// Angelic Vortex's, to be a page of the vortex's datapack directory.
func TestVortexPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "ai/boss/baium/BaiumAngelicVortex")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
