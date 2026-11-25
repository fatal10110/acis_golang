package bingo

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the family package with the union
// of its reference classes: literals, engine calls and hook shapes. The
// reference reads the lines of the board from a shared constant, which the
// port holds inline, and sizes its chance maps; the port names the variable a
// pick finds missing.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"quest.Q384_WarehouseKeepersPastime",
		"quest.Q386_StolenDignity",
	},
		"number 26", "number 5", "number 6", "number 7", "number 8",
		`string "bingo: no variable "`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
