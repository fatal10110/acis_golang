package valakas

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestHeartPagesAreInTheDatapack requires every page the package names, the
// Heart of Volcano's, to be a page of the Heart's datapack directory.
func TestHeartPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "ai/boss/valakas/HeartOfVolcano")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
