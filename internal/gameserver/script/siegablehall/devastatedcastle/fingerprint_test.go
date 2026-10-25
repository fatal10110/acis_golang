package devastatedcastle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// DevastatedCastle class. The reference names the hall (34), the shouts
// and lines and Dietrich's healer and buffer classes by their constants,
// which the port writes as their ids.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.siegablehall.DevastatedCastle"},
		"number 34",
		"number 1000275", "number 1000276", "number 1000277", "number 1000278", "number 1000279", "number 1000280",
		"number 16", "number 17", "number 30", "number 43", "number 51", "number 52",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
