package zaken

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class it ports: literals, engine calls and hook shapes. The reference
// brings a dead private back from a pooled task that first moves it, by
// then decayed, to a copy of Zaken's position given a new heading, then
// respawns it at its own point; the port asks the private's spawn for that
// respawn at the death, which also keeps a one-time private's spawn for
// it.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.boss.zaken.Zaken"},
		"call ThreadPool.schedule", "call WorldObject.setXYZ", "call SpawnLocation.setHeading", "call ASpawn.doRespawn")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
