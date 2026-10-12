package script

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// WorldNPC scans the world without copying it: looking up an absent NPC
// allocates nothing, and finding one allocates only its handle.
func TestWorldNPCDoesNotCopyTheWorld(t *testing.T) {
	w := world.New()
	for i := int32(0); i < 2048; i++ {
		w.AddObject(newNPCHandle(t, 1_000+i, 30_000+int(i%8)).combatant().(world.Tracked))
	}
	s := &Script{env: &Env{World: w}}

	if n := s.WorldNPC(30_003); n == nil || n.NpcID() != 30_003 {
		t.Fatalf("WorldNPC(30003) = %v, want an NPC of template 30003", n)
	}
	if n := s.WorldNPC(31_000); n != nil {
		t.Fatalf("WorldNPC(31000) = %v, want nil", n)
	}

	if allocs := testing.AllocsPerRun(100, func() { s.WorldNPC(31_000) }); allocs != 0 {
		t.Fatalf("WorldNPC of an absent NPC allocated %.0f times per call, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(100, func() { s.WorldNPC(30_003) }); allocs > 1 {
		t.Fatalf("WorldNPC of a present NPC allocated %.0f times per call, want at most its handle", allocs)
	}
}
