package npcs

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// stallWorld is the lifecycle fixture with the wolf and the grocer bound to
// a behavior whose timer hook, on the NPC's own queue, stops halfway until
// the test lets it go on.
type stallWorld struct {
	*lifeWorld
	// scripts holds each NPC's behavior, set by its created hook.
	scripts sync.Map // template id -> *script.Script
	// entered gets the template id of an NPC whose timer hook has stopped;
	// resume lets it go on.
	entered chan int32
	resume  chan struct{}
	// after is what the timer hook saw once it went on.
	after atomic.Value // string
}

func (w *stallWorld) behavior(id int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{id}, Hooks: script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				w.scripts.Store(id, s)
				w.log.add("stall", "created", id, e.NPC, "")
			},
			OnDecayed: func(_ *script.Script, e script.Decayed) { w.log.add("stall", "decayed", id, e.NPC, "") },
			OnTimer: func(_ *script.Script, e script.Timer) string {
				w.entered <- id
				<-w.resume
				// What a hook still running on the NPC's queue does once the
				// NPC is gone: speak, and take up an attack.
				w.after.Store(fmt.Sprintf("decayed=%v", e.NPC.Decayed()))
				e.NPC.Say("still here")
				if e.Player != nil {
					e.NPC.AddAttackDesire(e.Player, 1000)
				}
				return ""
			},
		}}
	}
}

func bootStall(t *testing.T, extra ...gameservertest.Option) *stallWorld {
	t.Helper()
	w := &stallWorld{entered: make(chan int32, 1), resume: make(chan struct{})}
	log := &lifeLog{}
	w.lifeWorld = &lifeWorld{log: log}
	kinds := map[int32]script.NPCKind{lifeWolfID: script.KindHostile, lifeGrocerID: script.KindFolk}
	list := []script.Listing{{Path: "ai.StallWolf"}, {Path: "ai.StallGrocer"}}
	catalog := script.Catalog{"ai.StallWolf": w.behavior(lifeWolfID), "ai.StallGrocer": w.behavior(lifeGrocerID)}
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(lifeTemplates()),
		gameservertest.WithNpcSpawns(lifeTable(t)),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithAITask(),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w.srv, w.player = srv, srv.SoleObjectID(t)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	w.burst = enterWorld(t, srv, c)
	log.srv.Store(srv)
	return w
}

// naming returns the frames whose subject, the object id right after the
// opcode, is id.
func naming(frames [][]byte, id int32) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if len(f) >= 5 && wire.NewReader(f[1:]).ReadInt32() == id {
			out = append(out, f)
		}
	}
	return out
}

// A script delete from another goroutine does not wait for the task the
// NPC's own queue is running: the NPC is out of the world, its decayed
// hooks run and its queue closed when the delete returns, while that task
// is still halfway through a hook. The hook then finds the NPC decayed, and
// nothing it does on the NPC's behalf, a chat line or an attack, reaches the
// client or the world: the client hears nothing more of the NPC after its
// DeleteObject. Monster and civilian alike, on both executors.
func TestScriptDeleteDoesNotWaitForTheNPCQueueTaskInFlight(t *testing.T) {
	t.Parallel()
	executors(t, func(t *testing.T, opts ...gameservertest.Option) {
		for _, id := range []int32{lifeWolfID, lifeGrocerID} {
			t.Run(fmt.Sprint(id), func(t *testing.T) {
				w := bootStall(t, opts...)
				w.log.take()
				obj, _ := w.srv.State.Object(w.player)
				pl := script.PlayerOf(obj.(attackable.Combatant))
				target := w.live(t, int(id))
				s, _ := w.scripts.Load(id)
				if !s.(*script.Script).StartTimer("stall", script.NPCOf(target), pl, 100*time.Millisecond) {
					t.Fatal("the stall timer did not start")
				}
				advanced := make(chan struct{})
				go func() {
					defer close(advanced)
					if w.srv.DrivesClock() {
						w.srv.Advance(t, 100*time.Millisecond)
					}
				}()
				if got := <-w.entered; got != id {
					t.Fatalf("stalled npc = %d, want %d", got, id)
				}

				script.NPCOf(target).DeleteMe()
				if got, want := w.log.take(), []string{fmt.Sprintf("stall decayed %d decayed=true inWorld=true", id)}; !slices.Equal(got, want) {
					t.Fatalf("delete hooks = %q, want %q", got, want)
				}
				if w.inWorld(int(id)) {
					t.Fatal("the NPC is still in the world when the delete returns")
				}
				if queueOf(target).Post(func() {}) {
					t.Fatal("the NPC's queue still takes tasks when the delete returns")
				}

				close(w.resume)
				<-advanced
				w.srv.Settle(t)
				if got := w.after.Load(); got != "decayed=true" {
					t.Fatalf("the hook in flight saw %v once it went on, want decayed=true", got)
				}
				before := w.srv.PlayerCurrentHP(t, w.player)
				if err := w.srv.AI.Tick(); err != nil {
					t.Fatalf("AI.Tick() = %v", err)
				}
				w.srv.Settle(t)
				if w.srv.DrivesClock() {
					w.srv.Advance(t, 5*time.Second)
				}
				w.srv.Settle(t)

				frames := drainFrames(t, w.srv.Client)
				got := naming(frames, target.ObjectID())
				if len(got) == 0 || got[0][0] != serverpackets.OpcodeDeleteObject {
					t.Fatalf("frames naming the NPC = %x, want its DeleteObject first", got)
				}
				if len(got) > 1 {
					t.Fatalf("frames naming the NPC after its DeleteObject = %x, want none", got[1:])
				}
				if after := w.srv.PlayerCurrentHP(t, w.player); after != before {
					t.Fatalf("player HP %d -> %d after the NPC was deleted", before, after)
				}
				if w.inWorld(int(id)) {
					t.Fatal("the deleted NPC came back into the world")
				}
			})
		}
	})
}

// Deleting a fighting monster from another goroutine, while its queue keeps
// running its AI ticks and hits, ends its fight when the delete returns: it
// is out of the world, decayed once, and lands no more hits however long the
// fight would have gone on. Raced many times under -race on both executors.
func TestScriptDeleteOffQueueEndsTheFightInFlight(t *testing.T) {
	t.Parallel()
	executors(t, func(t *testing.T, opts ...gameservertest.Option) {
		w := bootLife(t, append(opts, gameservertest.WithAITask())...)
		// A round on the wall clock lasts until the wolf's first hit lands.
		rounds := 10
		if !w.srv.DrivesClock() {
			rounds = 4
		}
		obj, _ := w.srv.State.Object(w.player)
		pl := script.PlayerOf(obj.(attackable.Combatant))
		decayed := fmt.Sprintf("recorder decayed %d decayed=true inWorld=true", lifeWolfID)
		for round := range rounds {
			w.srv.Settle(t)
			w.srv.AddPlayerHP(t, w.player, float64(w.srv.PlayerMaxHP(t, w.player)))
			wolf := w.live(t, lifeWolfID).(*npc.Hostile)
			onQueueOf(t, wolf, func() { script.NPCOf(wolf).AddAttackDesire(pl, 1000) })
			w.log.take()

			// The fight goes on, on the wolf's queue, until the delete is
			// done; the delete starts once the wolf has landed a hit.
			stop := make(chan struct{})
			going := make(chan struct{})
			fought := make(chan struct{})
			go func() {
				defer close(fought)
				hit := false
				for {
					select {
					case <-stop:
						return
					default:
					}
					_ = w.srv.AI.Tick()
					if w.srv.DrivesClock() {
						w.srv.Advance(t, 100*time.Millisecond)
					} else {
						w.srv.TickPositions()
					}
					if !hit && w.srv.PlayerCurrentHP(t, w.player) < w.srv.PlayerMaxHP(t, w.player) {
						hit = true
						close(going)
					}
				}
			}()
			select {
			case <-going:
			case <-time.After(30 * time.Second):
				t.Fatalf("round %d: the wolf never hit the character", round)
			}

			script.NPCOf(wolf).DeleteMe()
			if w.inWorld(lifeWolfID) {
				t.Fatalf("round %d: the wolf is still in the world when the delete returns", round)
			}
			if !wolf.Decayed() {
				t.Fatalf("round %d: the wolf is not decayed when the delete returns", round)
			}
			close(stop)
			<-fought
			w.srv.Settle(t)

			hp := w.srv.PlayerCurrentHP(t, w.player)
			_ = w.srv.AI.Tick()
			w.srv.Settle(t)
			if w.srv.DrivesClock() {
				w.srv.Advance(t, 3*time.Second)
			}
			w.srv.Settle(t)
			if got := w.srv.PlayerCurrentHP(t, w.player); got < hp {
				t.Fatalf("round %d: player HP %d -> %d after the wolf was deleted", round, hp, got)
			}
			n := 0
			for _, line := range w.log.take() {
				if line == decayed {
					n++
				}
			}
			if n != 1 {
				t.Fatalf("round %d: decayed hooks ran %d times, want 1", round, n)
			}
			if !w.srv.NpcRespawns.Cancel(lifeWolfKey) {
				t.Fatalf("round %d: the delete armed no respawn", round)
			}
			w.srv.NpcSpawns.Respawn(lifeWolfKey)
		}
	})
}
