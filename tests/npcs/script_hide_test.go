package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A script's decayMe hides an npcmaker's monster: it leaves the world with
// no decayed hook and its spawn is not told, so no respawn is armed, and
// the behavior timer bound to it stops and never fires. A second hide
// changes nothing.
func TestScriptDecayMeHidesWithoutDecay(t *testing.T) {
	t.Parallel()
	d := &decayTimers{id: lifeWolfID}
	srv := gameservertest.Boot(t,
		gameservertest.WithNPCs(lifeTemplates()),
		gameservertest.WithNpcSpawns(lifeTable(t)),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{lifeWolfID: script.KindHostile}, []script.Listing{{Path: "ai.DecayTimers"}}, script.Catalog{"ai.DecayTimers": d.script}),
	)
	wolf := liveOf(t, srv, lifeWolfID).(*npc.Hostile)
	handle := script.NPCOf(attackable.Combatant(wolf))
	s := d.self.Load()
	if s == nil || !s.HasTimer(decayTimerName, handle, nil) {
		t.Fatal("the created hook started no timer")
	}

	onQueueOf(t, wolf, func() { s.Spawner().DecayMe(handle) })
	if _, ok := srv.State.Object(wolf.ObjectID()); ok {
		t.Fatal("the hidden wolf is still in the world")
	}
	if got := d.seen(); len(got) != 0 {
		t.Fatalf("decayed hooks ran on the hide: %v", got)
	}
	if s.HasTimer(decayTimerName, handle, nil) {
		t.Fatal("the hidden wolf's behavior timer is still pending")
	}
	if srv.NpcRespawns.Tracked(lifeWolfKey) {
		t.Fatal("the hide armed the wolf's respawn")
	}
	s.Spawner().DecayMe(handle)
	srv.Advance(t, time.Hour)
	if n := d.fired.Load(); n != 0 {
		t.Fatalf("the hidden wolf's timer fired %d times", n)
	}
	if got := d.seen(); len(got) != 0 {
		t.Fatalf("decayed hooks ran after the hide: %v", got)
	}
}
