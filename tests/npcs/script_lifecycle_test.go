package npcs

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The lifecycle fixture's NPC templates, one npcmaker each.
const (
	lifeWolfID      = 21201 // default maker, respawns
	lifeHeldID      = 21202 // default maker held by an unlisted event
	lifeGrocerID    = 21203 // a mortal civilian, respawns
	lifeSelfDelID   = 21204 // its behavior deletes it as it is created
	lifeCompanionID = 21205 // spawned by the wolf's created hook
)

// The lifecycle fixture's spawn slot keys.
const (
	lifeWolfKey    = "life_wolf#0#0"
	lifeGrocerKey  = "life_grocer#0#0"
	lifeSelfDelKey = "life_selfdel#0#0"
)

// lifeSpawnlist declares the fixture's npcmakers, every NPC at a fixed
// point in sight of the character at (10, 20). The held maker has no
// event attribute, so it starts at boot, but its EventName memo is not in
// SpawnEvents: its NPC spawns and its maker deletes it at once.
const lifeSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="life_wolf" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21201" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="life_held" territory="field" maximumNpcs="1">
		<ai type="default_maker"><set name="EventName" val="christmas"/></ai>
		<npc id="21202" total="1" pos="50;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="life_grocer" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21203" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="life_selfdel" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21204" total="1" pos="70;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

func lifeTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "life.xml"), []byte(lifeSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

func lifeTemplates() *npc.Table {
	grocer := gameservertest.FolkTemplate("Merchant", lifeGrocerID)
	grocer.Undying = false
	return npc.NewTable([]*npc.Template{
		spawnMonster(lifeWolfID, "Wolf"),
		spawnMonster(lifeHeldID, "Held"),
		grocer,
		spawnMonster(lifeSelfDelID, "Self Deleter"),
		spawnMonster(lifeCompanionID, "Companion"),
	})
}

// lifeLog records the created, decayed and dying hooks of the fixture's
// scripts, one line each. Hooks run on the spawner's goroutine, the NPCs'
// queues and the engine queue, so lines is guarded by mu.
type lifeLog struct {
	mu    sync.Mutex
	lines []string
	// srv is the booted server, nil while Boot runs; a hook that runs once
	// it is set also says whether its NPC is still in the world.
	srv atomic.Pointer[gameservertest.Server]
	// spawner, once set, is what the wolf's created hook spawns its
	// companion with.
	spawner atomic.Pointer[script.Spawner]
}

func (l *lifeLog) add(who, what string, id int32, n *script.NPC, extra string) {
	line := fmt.Sprintf("%s %s %d decayed=%v", who, what, id, n.Decayed())
	if srv := l.srv.Load(); srv != nil {
		_, in := srv.State.Object(n.ObjectID())
		line += fmt.Sprintf(" inWorld=%v", in)
	}
	line += extra
	l.mu.Lock()
	l.lines = append(l.lines, line)
	l.mu.Unlock()
}

// take returns the lines since the last call.
func (l *lifeLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

// count returns how many lines are recorded.
func (l *lifeLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.lines)
}

// recorder is the behavior of the NPC id: the wolf, the held NPC or the
// grocer. The wolf's created hook spawns a companion once the spawner is
// set.
func (l *lifeLog) recorder(id int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{id}, Hooks: script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) {
				l.add("recorder", "created", id, e.NPC, "")
				if sp := l.spawner.Load(); sp != nil && id == lifeWolfID {
					sp.AddSpawn(lifeCompanionID, e.NPC, false, 0)
				}
			},
			OnDecayed: func(_ *script.Script, e script.Decayed) { l.add("recorder", "decayed", id, e.NPC, "") },
			OnMyDying: func(_ *script.Script, e script.MyDying) {
				l.add("recorder", "dying", id, e.NPC, fmt.Sprintf(" killer=%d", e.Killer.ObjectID()))
			},
		}}
	}
}

// watcher is a quest bound to the dying event of the NPC id and, for the
// wolf, to its created event; it is listed after the recorders.
func (l *lifeLog) watcher(id int32) func() script.Script {
	return func() script.Script {
		bind := script.Bindings{script.EventMyDying: {id}}
		if id == lifeWolfID {
			bind[script.EventCreated] = []int32{id}
		}
		return script.Script{Bind: bind, Hooks: script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) { l.add("watcher", "created", id, e.NPC, "") },
			OnMyDying: func(_ *script.Script, e script.MyDying) {
				l.add("watcher", "dying", id, e.NPC, fmt.Sprintf(" killer=%d", e.Killer.ObjectID()))
			},
		}}
	}
}

// selfDeleter deletes its NPC from its created hook, and records whether
// the NPC was gone once the delete returned.
func (l *lifeLog) selfDeleter() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{lifeSelfDelID}, Hooks: script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			l.add("selfdel", "created", lifeSelfDelID, e.NPC, "")
			e.NPC.DeleteMe()
			l.add("selfdel", "deleted", lifeSelfDelID, e.NPC, "")
		},
	}}
}

// lifeWorld is the booted lifecycle fixture with the character in the
// world.
type lifeWorld struct {
	srv    *gameservertest.Server
	log    *lifeLog
	burst  [][]byte
	player int32
}

func bootLife(t *testing.T, extra ...gameservertest.Option) *lifeWorld {
	t.Helper()
	log := &lifeLog{}
	kinds := map[int32]script.NPCKind{
		lifeWolfID: script.KindHostile, lifeHeldID: script.KindHostile, lifeGrocerID: script.KindFolk,
		lifeSelfDelID: script.KindHostile, lifeCompanionID: script.KindHostile,
	}
	list := []script.Listing{
		{Path: "ai.RecordWolf"},
		{Path: "ai.RecordHeld"},
		{Path: "ai.RecordGrocer"},
		{Path: "quest.WatchWolf"},
		{Path: "quest.WatchGrocer"},
		{Path: "ai.SelfDeleter"},
	}
	catalog := script.Catalog{
		"ai.RecordWolf":     log.recorder(lifeWolfID),
		"ai.RecordHeld":     log.recorder(lifeHeldID),
		"ai.RecordGrocer":   log.recorder(lifeGrocerID),
		"quest.WatchWolf":   log.watcher(lifeWolfID),
		"quest.WatchGrocer": log.watcher(lifeGrocerID),
		"ai.SelfDeleter":    log.selfDeleter,
	}
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(lifeTemplates()),
		gameservertest.WithNpcSpawns(lifeTable(t)),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &lifeWorld{srv: srv, log: log, player: srv.SoleObjectID(t)}
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	w.burst = enterWorld(t, srv, c)
	log.srv.Store(srv)
	return w
}

// live returns the one NPC of template id in the world.
func (w *lifeWorld) live(t *testing.T, id int) attackable.Combatant {
	t.Helper()
	return liveOf(t, w.srv, id).(attackable.Combatant)
}

// inWorld reports whether an NPC of template id is in the world.
func (w *lifeWorld) inWorld(id int) bool {
	for _, obj := range w.srv.State.Objects() {
		switch o := obj.(type) {
		case *npc.Hostile:
			if o.Instance.Template.ID == id {
				return true
			}
		case *npc.Folk:
			if o.Instance.Template.ID == id {
				return true
			}
		}
	}
	return false
}

// onQueueOf runs fn on the NPC's own queue and waits for it.
func onQueueOf(t *testing.T, n attackable.Combatant, fn func()) {
	t.Helper()
	var q interface{ Post(func()) bool }
	switch o := n.(type) {
	case *npc.Hostile:
		q = o.Queue()
	case *npc.Folk:
		q = o.Queue()
	}
	done := make(chan struct{})
	if !q.Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("npc queue is closed")
	}
	<-done
}

// executors runs fn on the inline executor and on the real pool.
func executors(t *testing.T, fn func(t *testing.T, opts ...gameservertest.Option)) {
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn(t, tc.opts...)
		})
	}
}

// At boot every NPC's created hooks run in list order as it enters the
// world, before its npcmaker's hook: the held maker's NPC is seen created
// alive, then decayed when its maker deletes it. An NPC its own created
// hook deletes is gone when the delete returns, and as that happens
// during its slot's first placement its maker is not told: no respawn is
// armed. The character is shown exactly the NPCs left standing.
func TestScriptCreatedHooksRunAtBootBeforeTheMakerHook(t *testing.T) {
	t.Parallel()
	executors(t, func(t *testing.T, opts ...gameservertest.Option) {
		w := bootLife(t, opts...)

		// The npcmakers start in name order.
		want := []string{
			"recorder created 21203 decayed=false",
			"recorder created 21202 decayed=false",
			"recorder decayed 21202 decayed=true",
			"selfdel created 21204 decayed=false",
			"selfdel deleted 21204 decayed=true",
			"recorder created 21201 decayed=false",
			"watcher created 21201 decayed=false",
		}
		if got := w.log.take(); !slices.Equal(got, want) {
			t.Fatalf("boot hooks =\n%q\nwant\n%q", got, want)
		}
		if got, want := shown(w.burst), []int{lifeWolfID, lifeGrocerID}; !slices.Equal(got, want) {
			t.Fatalf("NPCs shown at enter world = %v, want %v", got, want)
		}
		for _, id := range []int{lifeHeldID, lifeSelfDelID} {
			if w.inWorld(id) {
				t.Errorf("npc %d still in the world", id)
			}
		}
		if w.srv.NpcRespawns.Tracked(lifeSelfDelKey) {
			t.Fatal("an NPC deleted in its first placement armed its maker's respawn")
		}
	})
}

// A respawn runs the created hooks again, for the new NPC; an NPC its
// created hook deletes on a respawn is one of its spawn's NPCs by then, so
// its maker arms the next respawn.
func TestScriptCreatedHooksRunOnRespawn(t *testing.T) {
	t.Parallel()
	w := bootLife(t)
	w.log.take()

	old := w.live(t, lifeWolfID)
	old.(*npc.Hostile).DeleteMe()
	w.srv.Settle(t)
	if got, want := w.log.take(), []string{"recorder decayed 21201 decayed=true inWorld=true"}; !slices.Equal(got, want) {
		t.Fatalf("delete hooks = %q, want %q", got, want)
	}
	w.srv.NpcSpawns.Respawn(lifeWolfKey)
	w.srv.Settle(t)
	if got, want := w.log.take(), []string{
		"recorder created 21201 decayed=false inWorld=true",
		"watcher created 21201 decayed=false inWorld=true",
	}; !slices.Equal(got, want) {
		t.Fatalf("respawn hooks = %q, want %q", got, want)
	}
	if now := w.live(t, lifeWolfID); now.ObjectID() == old.ObjectID() {
		t.Fatal("the respawn reused the deleted NPC's object id")
	}

	w.srv.NpcSpawns.Respawn(lifeSelfDelKey)
	w.srv.Settle(t)
	if got, want := w.log.take(), []string{
		"selfdel created 21204 decayed=false inWorld=true",
		"selfdel deleted 21204 decayed=true inWorld=false",
	}; !slices.Equal(got, want) {
		t.Fatalf("self-deleting respawn hooks = %q, want %q", got, want)
	}
	if !w.srv.NpcRespawns.Tracked(lifeSelfDelKey) {
		t.Fatal("an NPC deleted on its respawn armed no respawn")
	}
}

// The dying hooks of every script bound to a dead NPC run three seconds
// after its death, in list order, with its killer, even though its corpse
// decayed and left the world in between: for a monster and for a mortal
// civilian NPC alike.
func TestScriptDyingHooksRunThreeSecondsAfterDeath(t *testing.T) {
	t.Parallel()
	for _, id := range []int{lifeWolfID, lifeGrocerID} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			executors(t, func(t *testing.T, opts ...gameservertest.Option) {
				w := bootLife(t, opts...)
				w.log.take()
				killer, _ := w.srv.State.Object(w.player)
				target := w.live(t, id)
				onQueueOf(t, target, func() {
					switch o := target.(type) {
					case *npc.Hostile:
						o.Kill(killer.(attackable.Combatant))
					case *npc.Folk:
						o.Kill(killer.(attackable.Combatant))
					}
				})
				onQueueOf(t, target, func() {
					switch o := target.(type) {
					case *npc.Hostile:
						o.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(o.ObjectID()))
					case *npc.Folk:
						o.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(o.ObjectID()))
					}
				})
				w.srv.Settle(t)
				if got, want := w.log.take(), []string{fmt.Sprintf("recorder decayed %d decayed=true inWorld=true", id)}; !slices.Equal(got, want) {
					t.Fatalf("hooks at the decay = %q, want %q", got, want)
				}
				dying := []string{
					fmt.Sprintf("recorder dying %d decayed=true inWorld=false killer=%d", id, w.player),
					fmt.Sprintf("watcher dying %d decayed=true inWorld=false killer=%d", id, w.player),
				}
				if !w.srv.DrivesClock() {
					w.srv.AdvanceUntil(t, "dying hooks", func() bool { return w.log.count() == len(dying) })
					if got := w.log.take(); !slices.Equal(got, dying) {
						t.Fatalf("dying hooks = %q, want %q", got, dying)
					}
					return
				}
				w.srv.Advance(t, 3*time.Second-time.Millisecond)
				if got := w.log.take(); len(got) != 0 {
					t.Fatalf("hooks before three seconds = %q, want none", got)
				}
				w.srv.Advance(t, time.Millisecond)
				if got := w.log.take(); !slices.Equal(got, dying) {
					t.Fatalf("dying hooks = %q, want %q", got, dying)
				}
			})
		})
	}
}

// A script delete takes the NPC out of the world before it returns: its
// decayed hook ran while it was still in the world, the handle reports it
// gone, the character saw it go, and its spawn armed its respawn. Monster
// and mortal civilian NPC alike.
func TestScriptDeleteIsSynchronous(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id  int
		key string
	}{{lifeWolfID, lifeWolfKey}, {lifeGrocerID, lifeGrocerKey}} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			t.Parallel()
			w := bootLife(t)
			w.log.take()
			target := w.live(t, tc.id)
			handle := script.NPCOf(target)

			handle.DeleteMe()
			if !handle.Decayed() {
				t.Fatal("the handle reports the NPC alive after the delete returned")
			}
			if _, in := w.srv.State.Object(target.ObjectID()); in {
				t.Fatal("the NPC is in the world after the delete returned")
			}
			if got, want := w.log.take(), []string{fmt.Sprintf("recorder decayed %d decayed=true inWorld=true", tc.id)}; !slices.Equal(got, want) {
				t.Fatalf("delete hooks = %q, want %q", got, want)
			}
			if !w.srv.NpcRespawns.Tracked(tc.key) {
				t.Fatal("the deleted NPC's spawn armed no respawn")
			}
			if !deletedIn(drainFrames(t, w.srv.Client), target.ObjectID()) {
				t.Fatal("the character was not told the NPC left")
			}
			handle.DeleteMe() // already gone: nothing happens
			if got := w.log.take(); len(got) != 0 {
				t.Fatalf("a second delete ran hooks %q", got)
			}
		})
	}
}

// //respawnall holds the population gate only while it swaps the spawn
// list: a created hook that spawns another NPC through the script spawner
// runs with no lock held, so the respawn finishes with both NPCs in the
// world, on both executors.
func TestScriptCreatedHookSpawnsDuringRespawnAll(t *testing.T) {
	t.Parallel()
	executors(t, func(t *testing.T, opts ...gameservertest.Option) {
		w := bootLife(t, opts...)
		w.log.spawner.Store(script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop()))
		npcs := w.srv.NpcSpawns

		npcs.DespawnAll()
		w.srv.Settle(t)
		w.log.take()
		done := make(chan struct{})
		go func() {
			defer close(done)
			npcs.RespawnAll(npcs.Spawns())
		}()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("RespawnAll did not return: a created hook waits on the population gate")
		}
		w.srv.Settle(t)
		for _, id := range []int{lifeWolfID, lifeCompanionID, lifeGrocerID} {
			if !w.inWorld(id) {
				t.Errorf("npc %d not in the world after the respawn", id)
			}
		}
	})
}
