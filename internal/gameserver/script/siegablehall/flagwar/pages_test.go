package flagwar

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestPagesAreInTheDatapack requires every page the flag war base names to
// be a page of the Bandit Stronghold's datapack directory, the first hall
// script deriving from it.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "siegablehall/BanditStronghold")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
