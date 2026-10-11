package monsterbehavior

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The use-skill boost
// constant, 100000, is declared by the reference classes and read by none
// of them nor of their children, so the port has no value to keep it in.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior",
	}, "number 100000")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
