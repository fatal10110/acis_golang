package script

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// keptNames returns the timers kept on the spawn slot of the world's NPC
// name, as "script name player" lines in list order.
func keptNames(w *timerWorld, name string) []string {
	h := keptTimers(w.fakes[name].scratch)
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, t := range h.list {
		out = append(out, t.s.Name+" "+t.key.name+" "+w.playerName(t.pl))
	}
	return out
}

// homesOf returns the NPCs whose spawn slots hold timers of the script
// labelled label, and how many of its timers it holds bound to none.
func homesOf(w *timerWorld, label string) (map[string]bool, int) {
	own := w.script(label).ownTimers
	own.mu.Lock()
	defer own.mu.Unlock()
	out := map[string]bool{}
	for name, f := range w.fakes {
		if _, ok := own.homes[keptTimers(f.scratch)]; ok {
			out[name] = true
		}
	}
	return out, len(own.loose)
}

// TestTimersBoundToAnNPCLiveOnItsSpawnSlot: a timer bound to an NPC is kept
// on its spawn slot, whatever its script and player; a script-wide cancel
// still reaches it, and the decay and detach removals take what they own
// from the slot.
func TestTimersBoundToAnNPCLiveOnItsSpawnSlot(t *testing.T) {
	w := newTimerWorld(t, listOf("ai.A", "quest.B"), func(w *timerWorld) Catalog {
		return Catalog{"ai.A": w.recorder("A", true), "quest.B": w.recorder("B", false)}
	})
	a, b := w.script("A"), w.script("B")
	n1, n2, p1 := w.npc("npc1"), w.npc("npc2"), w.player("p1")
	a.StartTimer("x", n1, nil, time.Second)
	a.StartTimer("x", n2, nil, time.Second)
	b.StartTimer("x", n1, p1, time.Second)
	a.StartTimer("y", nil, nil, time.Second)

	if got := keptNames(w, "npc1"); len(got) != 2 || got[0] != "A x none" || got[1] != "B x p1" {
		t.Fatalf("npc1 keeps %q, want [A x none, B x p1]", got)
	}
	if homes, loose := homesOf(w, "A"); !homes["npc1"] || !homes["npc2"] || loose != 1 {
		t.Fatalf("A is held by %v with %d timers bound to none, want npc1 and npc2 with 1", homes, loose)
	}

	a.CancelTimers(TimerName("x"))
	if a.HasTimer("x", n1, nil) || a.HasTimer("x", n2, nil) {
		t.Fatal("a script-wide cancel left a timer bound to an NPC")
	}
	if !b.HasTimer("x", n1, p1) || !a.HasTimer("y", nil, nil) {
		t.Fatal("a script-wide cancel took a timer of another script or name")
	}
	if homes, _ := homesOf(w, "A"); len(homes) != 0 {
		t.Fatalf("A still lists %v as holding its timers", homes)
	}

	// A quest timer bound to the NPC outlives its decay; its player's
	// departure takes it.
	w.r.timers.npcDecayed(w.fakes["npc1"].scratch)
	if !b.HasTimer("x", n1, p1) {
		t.Fatal("the decay took a quest timer")
	}
	w.r.timers.playerDetached(w.plf["p1"])
	if got := keptNames(w, "npc1"); len(got) != 0 {
		t.Fatalf("npc1 keeps %q after its player left", got)
	}
	if homes, _ := homesOf(w, "B"); len(homes) != 0 {
		t.Fatalf("B still lists %v as holding its timers", homes)
	}
	w.r.timers.mu.Lock()
	left := len(w.r.timers.byPlayer)
	w.r.timers.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d players still indexed", left)
	}

	w.advanceTo(2000)
	if got := w.take(); len(got) != 1 || got[0] != "fire A y npc=none player=none at=1000" {
		t.Fatalf("firings %q, want only A y", got)
	}
}

// TestNPCTimerRestartAllocatesOnlyTheTimer: once an NPC's spawn slot has
// kept a timer, starting and cancelling a timer bound to the NPC allocates
// the timer alone, even with no other timer pending on it: the slot's list
// is kept, and no index entry is made.
func TestNPCTimerRestartAllocatesOnlyTheTimer(t *testing.T) {
	w := newTimerWorld(t, listOf("ai.A"), func(w *timerWorld) Catalog {
		return Catalog{"ai.A": w.recorder("A", true)}
	})
	a, n := w.script("A"), w.npc("npc1")
	cycle := func() {
		a.StartTimer("once", n, nil, time.Minute)
		a.CancelTimer("once", n, nil)
	}
	cycle()
	got := testing.AllocsPerRun(100, cycle)
	q := w.fakes["npc1"].queue
	arm := testing.AllocsPerRun(100, func() { q.After(time.Minute, func() {}).Stop() })
	// The registry's own allocations are the timer and its firing closure.
	if want := 2 + arm; got > want {
		t.Fatalf("a start and cancel allocates %v times, want at most %v", got, want)
	}
}

// TestTimerIndexRaces: starts, firings, hook restarts and cancels, cancels
// of every kind, decays and departures racing on many NPCs, players and
// scripts leave every timer in exactly one home, and nothing behind once
// all are cancelled.
func TestTimerIndexRaces(t *testing.T) {
	pool := sim.NewPool(4, zerolog.Nop())
	pool.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pool.Stop(ctx)
	})
	var stopping atomic.Bool
	hook := func(s *Script, e Timer) string {
		if !stopping.Load() && e.Name == "again" {
			s.StartTimer("again", e.NPC, e.Player, time.Millisecond)
		}
		if !stopping.Load() && e.Name == "tick" {
			s.CancelTimer("tick", e.NPC, e.Player)
			s.StartTimerAtFixedRate("tick", e.NPC, e.Player, time.Millisecond, time.Millisecond)
		}
		return ""
	}
	catalog := Catalog{
		"ai.A": func() Script {
			return Script{Behavior: true, NPCs: []int32{timerNPCID}, Hooks: Hooks{OnTimer: hook, OnAttacked: func(*Script, Attacked) {}}}
		},
		"quest.B": func() Script { return Script{Hooks: Hooks{OnTimer: hook}} },
	}
	r := Build(listOf("ai.A", "quest.B"), catalog, RaiseAll(Config{KindOf: allTemplates, Log: zerolog.Nop(), Queue: pool.NewQueue("script-timers")}))
	scripts := []*Script{r.entries[0].script, r.entries[1].script}
	queues := []*sim.Queue{}
	var npcs []*NPC
	var scratches []*npc.Scratch
	for i := range 8 {
		f := &timerNPCFake{combatant: combatant{objectID: int32(100 + i)}, id: timerNPCID, queue: pool.NewQueue("n"), scratch: npc.NewScratch()}
		npcs = append(npcs, &NPC{self: f})
		scratches = append(scratches, f.scratch)
		queues = append(queues, f.queue)
	}
	npcs = append(npcs, nil)
	var pls []*Player
	for i := range 3 {
		f := &timerPlayerFake{combatant: combatant{objectID: int32(200 + i)}, queue: pool.NewQueue("p")}
		pls = append(pls, &Player{self: f})
		queues = append(queues, f.queue)
	}
	pls = append(pls, nil)
	names := []string{"once", "again", "tick"}

	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			rng := rand.New(rand.NewPCG(uint64(g), 7))
			for range 400 {
				s := scripts[rng.IntN(len(scripts))]
				n, p := npcs[rng.IntN(len(npcs))], pls[rng.IntN(len(pls))]
				name := names[rng.IntN(len(names))]
				switch rng.IntN(8) {
				case 0, 1:
					s.StartTimer(name, n, p, time.Duration(rng.IntN(2))*time.Millisecond)
				case 2:
					s.StartTimerAtFixedRate(name, n, p, 0, time.Millisecond)
				case 3:
					s.CancelTimer(name, n, p)
				case 4:
					s.CancelTimers(TimerNPC(n))
				case 5:
					s.CancelTimers(TimerName(name))
				case 6:
					r.timers.npcDecayed(scratches[rng.IntN(len(scratches))])
				case 7:
					if p != nil {
						r.timers.playerDetached(p.self)
					}
					s.HasTimer(name, n, p)
				}
			}
		})
	}
	wg.Wait()

	// Stop the restarts, let the firings under way end, then cancel all.
	stopping.Store(true)
	drain := func() {
		for _, q := range append(queues, r.queue) {
			done := make(chan struct{})
			q.Post(func() { close(done) })
			<-done
		}
	}
	drain()
	for _, s := range scripts {
		s.CancelTimers()
	}
	drain()

	for i, sc := range scratches {
		if h := keptTimers(sc); h != nil {
			h.mu.Lock()
			left := len(h.list)
			h.mu.Unlock()
			if left != 0 {
				t.Fatalf("npc %d keeps %d timers after every cancel", i, left)
			}
		}
	}
	for _, s := range scripts {
		s.ownTimers.mu.Lock()
		loose, homes := len(s.ownTimers.loose), len(s.ownTimers.homes)
		s.ownTimers.mu.Unlock()
		if loose+homes != 0 {
			t.Fatalf("%s keeps %d timers and %d homes after every cancel", s.Name, loose, homes)
		}
	}
	r.timers.mu.Lock()
	players := len(r.timers.byPlayer)
	r.timers.mu.Unlock()
	if players != 0 {
		t.Fatalf("%d players still indexed after every cancel", players)
	}
}
