package clanrep

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestPagesAreInTheDatapack requires every page each quest names to be a page
// of its own datapack directory. The raid pages of Q508 and Q509 are built
// from the raid number, as in the reference.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		file, dir string
		exempt    []string
	}{
		{"q508.go", "quest/Q508_AClansReputation", []string{"a.htm", "b.htm"}},
		{"q509.go", "quest/Q509_TheClansPrestige", []string{"a.htm", "b.htm"}},
		{"q510.go", "quest/Q510_AClansReputation", nil},
	} {
		t.Run(c.file, func(t *testing.T) {
			problems, err := scriptpages.CheckFiles(idx, ".", c.dir, []string{c.file}, c.exempt...)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range problems {
				t.Error(p)
			}
		})
	}
}
