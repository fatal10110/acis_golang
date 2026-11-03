package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestBlackjudgeMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestBlackjudgeMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("blackjudge", []string{"script.feature.BlackJudge"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestBlackjudgePagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestBlackjudgePagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "blackjudge", "feature/BlackJudge")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
