package fortressofthedead

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// FortressOfTheDead class. The reference names the hall (64) and the
// shouts by their constants, which the port writes as their ids; it reads
// the in-game hour off the game time in minutes (60), which the port reads
// directly; and it arms the siege's later start on the thread pool, which
// the port arms as the hall's siege step (Hall.ArmStart).
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.siegablehall.FortressOfTheDead"},
		"number 64", "number 1010624", "number 1010625", "number 1010636", "number 1010637", "number 1010638",
		"number 60", "call ThreadPool.schedule",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
