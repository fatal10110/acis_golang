package core

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids Core
// says are named constants in the reference, numbers in Go. The door id
// 20210001 is only registered by a constructor the script list never
// builds, so the Go package watches no door and names none.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.boss.core.Core"}, "number 1000001", "number 1000002", "number 1000003", "number 1000004", "number 1000005", "number 1000006", "number 20210001")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
