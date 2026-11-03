package fortressofresistance

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// FortressOfResistance class. The reference names the hall (21) by its
// constant, which the port writes as its id, and formats the siege date
// with a Java date pattern, which the port writes as Go's layout.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.siegablehall.FortressOfResistance"},
		"number 21", `string "yyyy-MM-dd HH:mm:ss"`, `string "2006-01-02 15:04:05"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
