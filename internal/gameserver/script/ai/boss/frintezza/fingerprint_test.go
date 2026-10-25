package frintezza

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes it ports: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.boss.frintezza.FollowerOfFrintezza",
		"script.ai.boss.frintezza.FollowerOfFrintezzaTransform",
		"script.ai.boss.frintezza.Frintezza",
		// Frintezza's on-screen song names are named constants in the
		// reference, numbers in Go.
	}, "number 1000522", "number 1000523", "number 1000524",
		"number 1000525", "number 1000526", "number 1000527")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
