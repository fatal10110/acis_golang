package ai

import (
	"testing"
)

// TestFolkRegionSleepClearsItsAggro pins Npc.onInactiveRegion for a
// civilian NPC: once the last player leaves its region, the NPC forgets its
// aggro list, as a monster does; while a player stays near, it keeps it.
func TestFolkRegionSleepClearsItsAggro(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	needsDrivenClock(t, srv)
	target := characterOf(t, srv)
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), offset(at, 40))
	if !f.Queue().Post(func() { f.Aggro().AddDamage(target, 5, 10) }) {
		t.Fatal("the folk's queue is closed")
	}
	srv.Settle(t)
	if hate := f.Aggro().Hate(target); hate != 10 {
		t.Fatalf("aggro against the character = %v, want 10", hate)
	}
	cycle(t, srv)
	if f.Aggro().IsEmpty() {
		t.Fatal("an AI cycle with a player in the region emptied the aggro list")
	}

	logOut(t, srv, f)
	if !f.Aggro().IsEmpty() {
		t.Fatalf("aggro list after its region went quiet = %+v, want empty", f.Aggro().Snapshot())
	}
}
