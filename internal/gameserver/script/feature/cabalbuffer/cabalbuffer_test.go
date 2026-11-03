package cabalbuffer

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids the
// priests say are named constants in the reference and numbers in Go.
func TestMatchesReferenceFingerprints(t *testing.T) {
	var exempt []string
	for _, chat := range [][]int32{preacherOfDoomRandomChat(), preacherOfDoomCastChat(), oratorOfRevelationsRandomChat(), oratorOfRevelationsCastChat()} {
		for _, id := range chat {
			exempt = append(exempt, fmt.Sprintf("number %d", id))
		}
	}
	problems, err := scriptfp.Check(".", []string{"script.feature.CabalBuffer"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
