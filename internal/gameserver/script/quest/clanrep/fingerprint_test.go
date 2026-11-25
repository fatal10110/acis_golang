package clanrep

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the family package with the union
// of its reference classes: literals, engine calls and hook shapes. The
// system message id is a named constant in the reference, and the reward
// columns its indices 2 and 3 read are struct fields here.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"quest.Q508_AClansReputation",
		"quest.Q509_TheClansPrestige",
		"quest.Q510_AClansReputation",
	}, "number 1777", "number 2", "number 3")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
