package script

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// homeWorld is a timer world with a behavior bound to the NPCs' template
// and a plain quest, each recording the queue its timer hook ran on.
func homeWorld(t *testing.T) (*timerWorld, *[]string) {
	t.Helper()
	var ran []string
	w := newTimerWorld(t, listOf("ai.Behavior", "quest.Quest"), func(w *timerWorld) Catalog {
		return Catalog{"ai.Behavior": w.recorder("Behavior", true), "quest.Quest": w.recorder("Quest", false)}
	})
	queues := map[string]*sim.Queue{"engine": w.engine}
	for name, f := range w.fakes {
		queues[name] = f.queue
	}
	for name, f := range w.plf {
		queues[name] = f.queue
	}
	w.onFire = func(_ *Script, e Timer) {
		for name, q := range queues {
			if owns(q) {
				ran = append(ran, e.Name+" on "+name)
			}
		}
	}
	return w, &ran
}

// owns reports whether the caller runs as q's owner.
func owns(q *sim.Queue) (ok bool) {
	defer func() { ok = recover() == nil }()
	sim.AssertOwner(q)
	return true
}

// TestTimerHomeQueue pins where a timer's hook runs: a behavior's timer on
// a live NPC of its template on the NPC's queue; otherwise on the bound
// player's queue; otherwise, or when the player is leaving, on the engine
// queue.
func TestTimerHomeQueue(t *testing.T) {
	w, ran := homeWorld(t)
	behavior, quest := w.script("Behavior"), w.script("Quest")
	npc1, p1, p2 := w.npc("npc1"), w.player("p1"), w.player("p2")

	behavior.StartTimer("alive", npc1, p1, time.Second)
	quest.StartTimer("quest", npc1, p1, time.Second)
	quest.StartTimer("npc-only", npc1, nil, time.Second)
	behavior.StartTimer("none", nil, nil, time.Second)
	w.plf["p2"].detaching.Store(true)
	quest.StartTimer("leaving", nil, p2, time.Second)
	w.fakes["npc2"].dead.Store(true)
	behavior.StartTimer("dead", w.npc("npc2"), p1, time.Second)
	w.advanceTo(1000)

	want := []string{"alive on npc1", "quest on p1", "npc-only on engine", "none on engine", "leaving on engine", "dead on p1"}
	if strings.Join(*ran, ",") != strings.Join(want, ",") {
		t.Fatalf("hooks ran %v, want %v", *ran, want)
	}
}

// TestTimersStopAtDecay: a behavior's timers bound to the NPC stop when it
// decays, wherever they run and whoever else they are bound to; a plain
// script's timer bound to the NPC survives the decay and fires, and so
// does a behavior timer bound to the other NPC.
func TestTimersStopAtDecay(t *testing.T) {
	w, _ := homeWorld(t)
	behavior, quest := w.script("Behavior"), w.script("Quest")
	npc1, p1 := w.npc("npc1"), w.player("p1")

	behavior.StartTimer("alive", npc1, nil, time.Second)
	behavior.StartTimerAtFixedRate("tick", npc1, p1, time.Second, time.Second)
	behavior.StartTimer("other", w.npc("npc2"), nil, time.Second)
	quest.StartTimer("quest", npc1, nil, time.Second)
	w.fakes["npc1"].dead.Store(true)
	behavior.StartTimer("corpse", npc1, nil, time.Second) // engine queue: the NPC is dead

	w.fakes["npc1"].decayed.Store(true)
	w.r.timers.npcDecayed(w.fakes["npc1"].scratch)
	for _, name := range []string{"alive", "tick", "corpse"} {
		if behavior.HasTimer(name, npc1, nil) || behavior.HasTimer(name, npc1, p1) {
			t.Fatalf("behavior timer %s still pending after the decay", name)
		}
	}
	w.advanceTo(3000)
	got := w.take()
	want := []string{"fire Behavior other npc=npc2 player=none at=1000", "fire Quest quest npc=npc1 player=none at=1000"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("fired:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// After the decay a behavior may start the key again: it is a new timer.
	if !behavior.StartTimer("alive", npc1, nil, time.Second) {
		t.Fatal("a restart after the decay was refused")
	}
}

// TestTimersStopAtDetach: every timer bound to a leaving player stops,
// whatever its script and NPC; timers of other players fire.
func TestTimersStopAtDetach(t *testing.T) {
	w, _ := homeWorld(t)
	behavior, quest := w.script("Behavior"), w.script("Quest")
	npc1, p1, p2 := w.npc("npc1"), w.player("p1"), w.player("p2")

	behavior.StartTimer("npc-queue", npc1, p1, time.Second)
	quest.StartTimerAtFixedRate("player-queue", nil, p1, time.Second, time.Second)
	quest.StartTimer("kept", nil, p2, time.Second)
	w.plf["p1"].detaching.Store(true)
	w.r.timers.playerDetached(w.plf["p1"])
	w.advanceTo(3000)
	if got := w.take(); len(got) != 1 || got[0] != "fire Quest kept npc=none player=p2 at=1000" {
		t.Fatalf("fired %q, want only p2's timer", got)
	}
	// A timer started for the departed player after its detach runs on the
	// engine queue and fires, as nothing stops it.
	if !quest.StartTimer("late", nil, p1, time.Second) {
		t.Fatal("start for a departed player refused")
	}
	w.advanceTo(5000)
	if got := w.take(); len(got) != 1 || got[0] != "fire Quest late npc=none player=p1 at=4000" {
		t.Fatalf("fired %q, want the late timer", got)
	}
}

// TestTimerStartRacingDecayIsRemoved: a behavior timer whose start chose
// the NPC's queue before a decay, but registered after the decay's
// removal, does not stay on the closed queue holding its key.
func TestTimerStartRacingDecayIsRemoved(t *testing.T) {
	w, _ := homeWorld(t)
	behavior := w.script("Behavior")
	f := w.fakes["npc1"]
	// The decay ran between the start's queue choice (the NPC is alive)
	// and its registration: the flag is set, the queue closed, the
	// removal already done.
	f.decayed.Store(true)
	f.queue.Close()
	if !behavior.StartTimer("late", w.npc("npc1"), nil, time.Second) {
		t.Fatal("start refused")
	}
	if behavior.HasTimer("late", w.npc("npc1"), nil) {
		t.Fatal("a start racing the decay left its timer registered")
	}
}

// TestTimerHookPanicIsRecovered: a panicking timer hook is logged with its
// stack and the registry goes on: a fixed-rate timer keeps its grid.
func TestTimerHookPanicIsRecovered(t *testing.T) {
	w, _ := homeWorld(t)
	quest := w.script("Quest")
	n := 0
	w.onFire = func(*Script, Timer) {
		n++
		if n == 1 {
			panic("boom")
		}
	}
	quest.StartTimerAtFixedRate("tick", nil, w.player("p1"), time.Second, time.Second)
	w.advanceTo(2000)
	if n != 2 {
		t.Fatalf("hook ran %d times, want 2", n)
	}
	if out := w.logs.String(); !strings.Contains(out, `"hook":"onTimer"`) || !strings.Contains(out, `"panic":"boom"`) || !strings.Contains(out, "timers_home_test.go") {
		t.Fatalf("panic not logged with the hook's stack: %s", out)
	}
}

// TestTimerAnswerIsNotShownYet: a timer bound to a player that answers
// with a page is logged, as showing an answer is the dialog path's; one
// bound to no player shows nothing and logs nothing.
func TestTimerAnswerIsNotShownYet(t *testing.T) {
	logs := &logBuffer{}
	clock := sim.NewInline(time.UnixMilli(0))
	catalog := Catalog{"quest.Pages": func() Script {
		return Script{Hooks: Hooks{OnTimer: func(*Script, Timer) string { return "page.htm" }}}
	}}
	r := Build(listOf("quest.Pages"), catalog, Config{KindOf: allTemplates, Log: zerolog.New(logs), Queue: clock.NewQueue("script-timers")})
	s := r.entries[0].script
	p := &Player{self: &timerPlayerFake{combatant: combatant{objectID: 1}, queue: clock.NewQueue("p")}}
	s.StartTimer("none", nil, nil, 0)
	clock.Advance(0)
	if logs.String() != "" && strings.Contains(logs.String(), "not shown") {
		t.Fatalf("a timer bound to no player logged its answer: %s", logs.String())
	}
	s.StartTimer("player", nil, p, 0)
	clock.Advance(0)
	if !strings.Contains(logs.String(), `"timer":"player","message":"script: timer answer not shown"`) {
		t.Fatalf("answer for a player not logged: %s", logs.String())
	}
}

// TestBehaviorTimersNeedTheDecayRemoval: a behavior that sets the timer
// hook registers only on NPC kinds whose decay stops its timers; a plain
// script's timers need no such seam.
func TestBehaviorTimersNeedTheDecayRemoval(t *testing.T) {
	onTimer := func(*Script, Timer) string { return "" }
	catalog := Catalog{
		"ai.Folk": func() Script {
			return Script{Behavior: true, NPCs: []int32{2}, Hooks: Hooks{OnTimer: onTimer, OnAttacked: func(*Script, Attacked) {}}}
		},
		"ai.Hostile": func() Script {
			return Script{Behavior: true, NPCs: []int32{1}, Hooks: Hooks{OnTimer: onTimer, OnAttacked: func(*Script, Attacked) {}}}
		},
		"quest.Folk": func() Script { return Script{Bind: Bindings{EventAttacked: {2}}, Hooks: Hooks{OnTimer: onTimer}} },
	}
	kindOf := func(id int32) (NPCKind, bool) {
		if id == 2 {
			return KindFolk, true
		}
		return KindHostile, true
	}
	// Attacked is taken as raised on every kind; the timer hook is raised
	// on every production kind but the civilian one, standing for a kind
	// whose decay does not stop the timers.
	if raisedHooks[hookTimer]&kinds(KindFolk) == 0 {
		t.Fatal("the timer hook is not raised on civilian NPCs, whose decay stops their timers")
	}
	raises := func(h hook, k NPCKind) bool {
		return h == hookAttacked || h == hookTimer && k != KindFolk && raisedHooks[h]&kinds(k) != 0
	}
	logs := &logBuffer{}
	r := Build(listOf("ai.Folk", "ai.Hostile", "quest.Folk"), catalog, Config{KindOf: kindOf, Log: zerolog.New(logs), raises: raises})
	states := []entryState{r.entries[0].state, r.entries[1].state, r.entries[2].state}
	if states[0] != entryRefused || states[1] != entryRegistered || states[2] != entryRegistered {
		t.Fatalf("entry states = %v, want the folk behavior refused and the rest registered", states)
	}
	if !strings.Contains(logs.String(), "behavior timers on npc 2") {
		t.Fatalf("refusal not logged: %s", logs.String())
	}
}

// TestConcurrentStartsOfOneKey: starts of one key racing on many
// goroutines leave exactly one timer, which fires once; cancels and
// decays racing with firings leave the registry consistent.
func TestConcurrentStartsOfOneKey(t *testing.T) {
	pool := sim.NewPool(4, zerolog.Nop())
	pool.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pool.Stop(ctx)
	})
	var fired atomic.Int32
	done := make(chan struct{}, 64)
	catalog := Catalog{"quest.Race": func() Script {
		return Script{Hooks: Hooks{OnTimer: func(s *Script, e Timer) string {
			fired.Add(1)
			select {
			case done <- struct{}{}:
			default:
			}
			return ""
		}}}
	}}
	r := Build(listOf("quest.Race"), catalog, Config{KindOf: allTemplates, Log: zerolog.Nop(), Queue: pool.NewQueue("script-timers")})
	s := r.entries[0].script
	p := &Player{self: &timerPlayerFake{combatant: combatant{objectID: 1}, queue: pool.NewQueue("p")}}

	const racers = 16
	var started atomic.Int32
	var wg sync.WaitGroup
	for range racers {
		wg.Go(func() {
			if s.StartTimer("once", nil, p, 10*time.Millisecond) {
				started.Add(1)
			}
		})
	}
	wg.Wait()
	if started.Load() != 1 {
		t.Fatalf("%d starts of one key succeeded, want 1", started.Load())
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the timer never fired")
	}

	// A fixed-rate timer firing while it is cancelled and its player
	// detaches from other goroutines.
	s.StartTimerAtFixedRate("tick", nil, p, 0, time.Millisecond)
	<-done
	wg.Go(func() { s.CancelTimers(TimerName("tick")) })
	wg.Go(func() { r.timers.playerDetached(p.self) })
	wg.Wait()
	if s.HasTimer("tick", nil, p) {
		t.Fatal("a cancelled timer is still pending")
	}
	// A firing already running ends without arming another.
	barrier := make(chan struct{})
	p.self.(*timerPlayerFake).queue.Post(func() { close(barrier) })
	<-barrier
	r.timers.mu.Lock()
	left := len(r.timers.byScript) + len(r.timers.byPlayer)
	r.timers.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d timer index entries left after the cancel", left)
	}
}
