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
		// The transform's HP stages 30022 and 30023 sit behind its 75%
		// threshold in first-match chains, so they are never reported.
	}, "number 30022", "number 30023")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
