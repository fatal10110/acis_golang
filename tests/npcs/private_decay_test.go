package npcs

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// A one-time private whose master died first still leaves that master's
// privates when it decays: the master's death let it go, but it stays the
// master's private until then.
func TestOneTimePrivateDecayLeavesItsDeadMaster(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	boss := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)
	if w.spawner.CreateOnePrivate(master, spawnScoutID, 0) == nil {
		t.Fatal("no private created")
	}
	scout := spawnedNPC(t, w, spawnScoutID).(*npc.Hostile)

	if !boss.Die(nil, nil) {
		t.Fatal("master did not die")
	}
	if scout.Master() != nil || scout.SpawnMaster() != boss || !slices.Contains(boss.Minions(), scout) {
		t.Fatal("the master's death did not let its private go while keeping it")
	}
	if !scout.Die(nil, nil) {
		t.Fatal("private did not die")
	}
	scoutID := scout.ObjectID()
	scout.Queue().Post(func() {
		scout.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(scoutID))
	})
	w.srv.Settle(t)
	if slices.Contains(boss.Minions(), scout) {
		t.Fatal("the decayed one-time private is still among its dead master's privates")
	}
}
