package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A spawn's respawn gives the object id of the NPC that left it back to the
// allocator as it places the next one on a fresh id, so a spawn respawning
// for ever holds one id at a time (#4784).

const (
	idReleaseWolfID = 21993
	idReleaseSlot   = "id_release#0#0"
	idReleaseList   = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="id_release_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="id_release" territory="id_release_field" maximumNpcs="1">
		<npc id="21993" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

func TestRespawnGivesTheLeftNPCsObjectIDBack(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(idReleaseWolfID)),
		gameservertest.WithNpcSpawns(batch1Table(t, idReleaseList)),
	)
	mw := &makerWorld{srv: w.srv}
	inUse := w.srv.ObjectIDsInUse()
	seen := map[int32]bool{}
	for round := range 5 {
		wolves := mw.inWorld(idReleaseWolfID)
		if len(wolves) != 1 {
			t.Fatalf("round %d: wolves = %d, want 1", round, len(wolves))
		}
		seen[wolves[0].ObjectID()] = true
		wolves[0].DeleteMe()
		w.srv.Settle(t)
		if got := w.srv.ObjectIDsInUse(); got != inUse {
			t.Fatalf("round %d, the wolf left: object ids in use = %d, want %d: it keeps its id until the respawn", round, got, inUse)
		}
		if !w.srv.RespawnArmed(t, idReleaseSlot) {
			t.Fatalf("round %d, the wolf left: no respawn armed", round)
		}
		w.srv.RespawnSlot(t, idReleaseSlot)
		w.srv.Settle(t)
		wolves = mw.inWorld(idReleaseWolfID)
		if len(wolves) != 1 {
			t.Fatalf("round %d, respawned: wolves = %d, want 1", round, len(wolves))
		}
		if id := wolves[0].ObjectID(); seen[id] {
			t.Fatalf("round %d: the respawn reused object id %d", round, id)
		}
		if got := w.srv.ObjectIDsInUse(); got != inUse {
			t.Fatalf("round %d, respawned: object ids in use = %d, want %d", round, got, inUse)
		}
	}
}
