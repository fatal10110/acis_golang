package sailren

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestStatuePagesAreInTheDatapack requires every page the package names,
// the statue's, to be a page of the statue's datapack directory.
func TestStatuePagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "ai/boss/sailren/StatueOfShilen")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
