package royalrush

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The reference names the
// charms' shouts and the trigger boxes' line by their NpcString constants;
// the port writes their ids.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushDefaultNpc",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAfflict",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTrasureBox",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAreaController",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushKeyBox",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBoxBase",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox1",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox2",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox3",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox4",
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox5",
	},
		"number 1010472", "number 1010473", "number 1010474", "number 1010475", "number 1010476", "number 1010477",
		"number 1010478", "number 1010479", "number 1010480", "number 1010481", "number 1010482", "number 1010552",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
