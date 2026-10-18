package defaultnpc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The private creation
// loop, with each private's respawn delay, is Spawner.CreatePrivates, and the "-" split of a skill parameter is
// inside NPC.SkillParam.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.individual.DefaultNpc"},
		"call Quest.createOnePrivate", "call ASpawn.setRespawnDelay", `string "-"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
