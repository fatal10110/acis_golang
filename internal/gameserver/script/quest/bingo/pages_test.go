package bingo

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestPagesAreInTheDatapack requires every page each quest names to be a page
// of its own datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ file, dir string }{
		{"q384.go", "quest/Q384_WarehouseKeepersPastime"},
		{"q386.go", "quest/Q386_StolenDignity"},
	} {
		t.Run(c.file, func(t *testing.T) {
			problems, err := scriptpages.CheckFiles(idx, ".", c.dir, []string{c.file})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range problems {
				t.Error(p)
			}
		})
	}
}
