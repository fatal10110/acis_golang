package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A maker that deletes its group, or a spawn that does not respawn, lets
// an NPC go from its spawn's list; a respawn the maker arms for it anyway
// still brings it back (#4038).

const (
	orphanBossID  = 21993
	orphanMobID   = 21994
	orphanOnceID  = 21995
	orphanSpawnls = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="orphan_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="orphan_m" territory="orphan_field" maximumNpcs="2">
		<ai type="orphan_probe"/>
		<npc id="21993" total="1" pos="40;20;30;0" respawn="60sec"/>
		<npc id="21994" total="1" pos="60;20;30;0" respawn="30sec"/>
	</npcmaker>
	<npcmaker name="orphan_once" territory="orphan_field" maximumNpcs="1">
		<ai type="orphan_probe"/>
		<npc id="21995" total="1" pos="80;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`
)

// orphanProbe is a fixture maker type: it starts every spawn full, logs
// each hook with its group's alive count and its spawn's spawned count,
// and answers the boss leaving with a DeleteAll and a 10 second respawn of
// the boss, and the respawn-less NPC leaving with a 10 second respawn.
// Script event "delete" runs a DeleteAll; each script event then logs the
// alive count and, per spawn, its spawned and decayed counts and how many
// NPCs it lists.
type orphanProbe struct {
	mu  sync.Mutex
	log []string
}

func (p *orphanProbe) add(format string, args ...any) {
	p.mu.Lock()
	p.log = append(p.log, fmt.Sprintf(format, args...))
	p.mu.Unlock()
}

func (p *orphanProbe) expect(t *testing.T, step string, want ...string) {
	t.Helper()
	p.mu.Lock()
	got := p.log
	p.log = nil
	p.mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: maker hooks = %q, want %q", step, got, want)
	}
}

func (p *orphanProbe) catalog() script.MakerCatalog {
	hook := func(what string, e script.MakerNPC) {
		p.add("%s %s:%d alive=%d spawned=%d", what, e.Group.Maker().Name, e.Spawn.Entry().NPCID, e.Group.Alive(), e.Spawn.Spawned())
	}
	return script.MakerCatalog{"orphan_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				for _, s := range e.Group.Spawns() {
					for s.Spawned() < s.Total() {
						s.Spawn()
					}
				}
			},
			OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) { hook("created", e) },
			OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
				hook("deleted", e)
				switch e.Spawn.Entry().NPCID {
				case orphanBossID:
					e.Group.DeleteAll()
					e.NPC.ScheduleRespawn(10 * time.Second)
				case orphanOnceID:
					e.NPC.ScheduleRespawn(10 * time.Second)
				}
			},
			OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
				if e.Name == "delete" {
					e.Group.DeleteAll()
				}
				line := fmt.Sprintf("%s %s alive=%d", e.Name, e.Group.Maker().Name, e.Group.Alive())
				for _, s := range e.Group.Spawns() {
					line += fmt.Sprintf(" %d:%d/%d/%d", s.Entry().NPCID, s.Spawned(), s.Decayed(), len(s.NPCs()))
				}
				p.add("%s", line)
			},
		}}
	}}
}

// The boss leaving deletes its group and arms its own respawn: 10 seconds
// later it is back and the maker hears it created, its group counting it
// alive though its spawn no longer lists it. A later DeleteAll leaves it
// in the world and counts it no more; its leaving after that runs no maker
// hook, takes the alive count below zero, and its slot goes. A spawn
// that does not respawn lets its NPC go the same way when the maker arms
// a respawn for it.
func TestMakerRespawnBringsBackTheNPCItsSpawnLetGo(t *testing.T) {
	t.Parallel()
	probe := &orphanProbe{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(orphanBossID, orphanMobID, orphanOnceID)),
		gameservertest.WithNpcSpawns(batch1Table(t, orphanSpawnls)),
		gameservertest.WithMakers(probe.catalog()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, boss, mob, once int) {
		t.Helper()
		for id, want := range map[int]int{orphanBossID: boss, orphanMobID: mob, orphanOnceID: once} {
			if got := len(mw.inWorld(id)); got != want {
				t.Fatalf("%s: NPCs of %d = %d, want %d", step, id, got, want)
			}
		}
	}
	event := func(maker, name string) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent(maker, name, 0, 0) {
			t.Fatalf("npcmaker %s not found by name", maker)
		}
		w.srv.Settle(t)
	}
	// The harness never fires a respawn timer: respawn brings an armed one
	// due.
	respawn := func(step, key string) {
		t.Helper()
		if !w.srv.NpcRespawns.Cancel(key) {
			t.Fatalf("%s: no respawn armed for %s", step, key)
		}
		w.srv.NpcSpawns.Respawn(key)
		w.srv.Settle(t)
	}
	count("boot", 1, 1, 1)
	probe.expect(t, "boot",
		"created orphan_m:21993 alive=1 spawned=1",
		"created orphan_m:21994 alive=2 spawned=1",
		"created orphan_once:21995 alive=1 spawned=1")
	drainFrames(t, w.c)

	boss := mw.inWorld(orphanBossID)[0]
	mob := mw.inWorld(orphanMobID)[0]
	boss.DeleteMe()
	w.srv.Settle(t)
	count("the boss left", 0, 0, 1)
	probe.expect(t, "the boss left", "deleted orphan_m:21993 alive=1 spawned=0")
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, objectIDs([]*npc.Hostile{boss, mob})) {
		t.Fatalf("the boss left: deleted objects = %v, want the boss and the mob", got)
	}
	if w.srv.NpcRespawns.Tracked("orphan_m#1#0") {
		t.Fatal("the boss left: the deleted mob's respawn is still armed")
	}

	respawn("the boss's respawn", "orphan_m#0#0")
	count("the boss respawned", 1, 0, 1)
	probe.expect(t, "the boss respawned", "created orphan_m:21993 alive=1 spawned=0")
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{orphanBossID}) {
		t.Fatalf("the boss respawned: NPCs shown = %v, want [%d]", got, orphanBossID)
	}
	event("orphan_m", "counts")
	probe.expect(t, "the boss respawned", "counts orphan_m alive=1 21993:0/0/0 21994:0/0/0")

	event("orphan_m", "delete")
	count("a second DeleteAll", 1, 0, 1)
	probe.expect(t, "a second DeleteAll", "delete orphan_m alive=0 21993:0/0/0 21994:0/0/0")
	if got := deleted(drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("a second DeleteAll: deleted objects = %v, want none", got)
	}

	mw.inWorld(orphanBossID)[0].DeleteMe()
	w.srv.Settle(t)
	event("orphan_m", "counts")
	probe.expect(t, "the respawned boss left", "counts orphan_m alive=-1 21993:0/0/0 21994:0/0/0")
	if w.srv.NpcRespawns.Tracked("orphan_m#0#0") {
		t.Fatal("the respawned boss left: a respawn is armed for it")
	}
	w.srv.NpcSpawns.Respawn("orphan_m#0#0")
	w.srv.Settle(t)
	count("its slot gone", 0, 0, 1)
	probe.expect(t, "its slot gone")

	once := mw.inWorld(orphanOnceID)[0]
	once.DeleteMe()
	w.srv.Settle(t)
	probe.expect(t, "the respawn-less NPC left", "deleted orphan_once:21995 alive=0 spawned=0")
	respawn("the respawn-less NPC's respawn", "orphan_once#0#0")
	count("the respawn-less NPC respawned", 0, 0, 1)
	probe.expect(t, "the respawn-less NPC respawned", "created orphan_once:21995 alive=1 spawned=0")
	event("orphan_once", "counts")
	probe.expect(t, "the respawn-less NPC respawned", "counts orphan_once alive=1 21995:0/0/0")
	mw.inWorld(orphanOnceID)[0].DeleteMe()
	w.srv.Settle(t)
	event("orphan_once", "counts")
	probe.expect(t, "the respawned NPC left", "counts orphan_once alive=0 21995:0/0/0")
}
