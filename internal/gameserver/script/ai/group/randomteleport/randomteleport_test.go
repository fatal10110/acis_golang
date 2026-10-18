package randomteleport

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids the
// Blacksmith of Mammon shouts are named constants in the reference and
// numbers in Go.
func TestMatchesReferenceFingerprints(t *testing.T) {
	var exempt []string
	for _, id := range bomShouts() {
		exempt = append(exempt, fmt.Sprintf("number %d", id))
	}
	problems, err := scriptfp.Check(".", []string{"script.ai.group.RandomTeleport"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
