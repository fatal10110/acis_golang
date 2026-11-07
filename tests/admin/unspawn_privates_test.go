package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// TestAdminUnspawnAllPlacesNoPrivateForGoneMaster: a master //unspawnall
// took out gets no private afterwards (its created hooks, or a hook on its
// queue, may still ask for one once the unspawn ran): the private stays out
// of the world and no spawn slot is left behind for it.
func TestAdminUnspawnAllPlacesNoPrivateForGoneMaster(t *testing.T) {
	t.Parallel()
	srv, _ := bootSpawnAdmin(t, wolfDen(t))
	var master *npc.Hostile
	for _, obj := range npcsOf(srv, wolfID) {
		if h := obj.(*npc.Hostile); h.SpawnMaster() == nil {
			master = h
		}
	}
	if master == nil {
		t.Fatal("wolf den has no master")
	}

	frames := exchange(t, srv.Client, encodeBuildCmd("unspawnall"))
	if got := noticesIn(frames); len(got) != 1 {
		t.Fatalf("game master messages = %q, want the unspawn notice", got)
	}
	private, err := srv.NpcSpawns.CreatePrivate(master, wolfID, nil, 0, 0, [3]int32{})
	if err != nil || private != nil {
		t.Fatalf("CreatePrivate for a master //unspawnall took out placed %v (err %v); want nothing placed and no error", private != nil, err)
	}

	srv.Settle(t)
	if got := allNpcs(srv); len(got) != 0 {
		t.Fatalf("npcs left in the world: %v", got)
	}
	if n := srv.RespawnsArmed(); n != 0 || srv.NpcSpawns.LiveCount() != 0 {
		t.Fatalf("respawns armed %d, live %d; want neither", n, srv.NpcSpawns.LiveCount())
	}
}
