package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A database-tracked spawn keeps a single NPC of its own; an NPC it let go
// that a respawn brings back is not that NPC, so the spawn still places
// one of its own beside it, whichever comes first (#4038).

const (
	orphanDBBossID  = 21992
	orphanDBSpawnls = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="orphan_db_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="orphan_db" territory="orphan_db_field" maximumNpcs="1">
		<ai type="orphan_db_probe"/>
		<npc id="21992" total="1" pos="40;20;30;0" respawn="60sec" dbName="orphan_db_boss"/>
	</npcmaker>
</list>`
)

// orphanDBProbe is a fixture maker type: it starts its spawn, logs each
// created and deleted hook with its group's alive count and its spawn's
// spawned count, and answers its NPC leaving with a DeleteAll, a spawn of
// a new NPC, then a 10 second respawn of the NPC that left. Script event
// "delete" runs a DeleteAll and "spawn" asks the spawn for one more NPC.
type orphanDBProbe struct {
	mu  sync.Mutex
	log []string
}

func (p *orphanDBProbe) add(format string, args ...any) {
	p.mu.Lock()
	p.log = append(p.log, fmt.Sprintf(format, args...))
	p.mu.Unlock()
}

func (p *orphanDBProbe) expect(t *testing.T, step string, want ...string) {
	t.Helper()
	p.mu.Lock()
	got := p.log
	p.log = nil
	p.mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: maker hooks = %q, want %q", step, got, want)
	}
}

func (p *orphanDBProbe) catalog() script.MakerCatalog {
	hook := func(what string, e script.MakerNPC) {
		p.add("%s alive=%d spawned=%d", what, e.Group.Alive(), e.Spawn.Spawned())
	}
	return script.MakerCatalog{"orphan_db_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				e.Group.Spawns()[0].Spawn()
			},
			OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) { hook("created", e) },
			OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
				hook("deleted", e)
				e.Group.DeleteAll()
				e.Spawn.Spawn()
				e.NPC.ScheduleRespawn(10 * time.Second)
			},
			OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
				switch e.Name {
				case "delete":
					e.Group.DeleteAll()
				case "spawn":
					e.Group.Spawns()[0].Spawn()
				}
				p.add("%s alive=%d", e.Name, e.Group.Alive())
			},
		}}
	}}
}

// The boss leaving deletes its group, has its spawn place a new boss, and
// arms its own respawn: the respawn is its own, not the new boss's, and
// brings it back beside the new one. With the respawned boss still in the
// world, a DeleteAll takes the spawn's boss away and the spawn places a
// new one again.
func TestMakerRespawnOfADatabaseTrackedNPCLeavesItsSpawnFree(t *testing.T) {
	t.Parallel()
	probe := &orphanDBProbe{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(orphanDBBossID)),
		gameservertest.WithNpcSpawns(batch1Table(t, orphanDBSpawnls)),
		gameservertest.WithMakers(probe.catalog()),
	)
	mw := &makerWorld{srv: w.srv}
	bosses := func(step string, want int) {
		t.Helper()
		if got := len(mw.inWorld(orphanDBBossID)); got != want {
			t.Fatalf("%s: bosses in the world = %d, want %d", step, got, want)
		}
	}
	event := func(name string) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent("orphan_db", name, 0, 0) {
			t.Fatal("npcmaker orphan_db not found by name")
		}
		w.srv.Settle(t)
	}
	bosses("boot", 1)
	probe.expect(t, "boot", "created alive=1 spawned=1")

	first := mw.inWorld(orphanDBBossID)[0]
	first.DeleteMe()
	w.srv.Settle(t)
	bosses("the boss left", 1)
	probe.expect(t, "the boss left",
		"deleted alive=0 spawned=0",
		"created alive=1 spawned=1")
	if second := mw.inWorld(orphanDBBossID)[0]; second.ObjectID() == first.ObjectID() {
		t.Fatal("the boss left: the spawn's new boss is the one that left")
	}
	// The spawn's slot key is its database name; the respawn the old
	// boss armed sits on a key of its own.
	if w.srv.NpcRespawns.Tracked("orphan_db_boss") {
		t.Fatal("the boss left: the old boss's respawn was armed on the new boss's slot")
	}
	const orphan = "orphan_db_boss~1"
	if !w.srv.NpcRespawns.Cancel(orphan) {
		t.Fatalf("the boss left: no respawn armed for %s", orphan)
	}
	// The harness never fires a respawn timer: bring it due by hand.
	w.srv.NpcSpawns.Respawn(orphan)
	w.srv.Settle(t)
	bosses("the old boss respawned", 2)
	probe.expect(t, "the old boss respawned", "created alive=2 spawned=1")

	event("delete")
	bosses("a DeleteAll", 1)
	probe.expect(t, "a DeleteAll", "delete alive=0")
	event("spawn")
	bosses("the spawn placed anew", 2)
	probe.expect(t, "the spawn placed anew", "created alive=1 spawned=1", "spawn alive=1")
}
