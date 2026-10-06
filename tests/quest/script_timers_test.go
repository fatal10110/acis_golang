package quest

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// timerWolfID is the monster template the timer behavior is bound to.
const timerWolfID = 20120

// timerSound is what the plain timer script plays to the player its timer
// is bound to, so a firing reaches the client.
const timerSound = "ItemSound.quest_middle"

// timerLog records the firings of the timer scripts. Hooks run on their
// timers' home queues, so it is guarded by mu.
type timerLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *timerLog) record(label string, e script.Timer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf("%s %s npc=%s player=%s", label, e.Name, handleID(e.NPC), handleID(e.Player)))
}

func handleID(c script.Creature) string {
	if c == nil || c.ObjectID() == 0 {
		return "none"
	}
	return fmt.Sprint(c.ObjectID())
}

func (l *timerLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

// bootTimers boots one character with a hostile template, the timer
// behavior bound to it and the plain timer script, and returns the
// server, the character's id and a spawner.
func bootTimers(t *testing.T, log *timerLog, opts ...gameservertest.Option) (*gameservertest.Server, int32, *script.Spawner) {
	t.Helper()
	wolf := &npc.Template{
		ID: timerWolfID, TemplateID: timerWolfID, Type: "Monster", Name: "Wolf", Level: 1, HPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10,
	}
	catalog := script.Catalog{
		"ai.TimerWolf": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{timerWolfID}, Hooks: script.Hooks{
				OnAttacked: func(*script.Script, script.Attacked) {},
				OnTimer: func(_ *script.Script, e script.Timer) string {
					log.record("TimerWolf", e)
					return ""
				},
			}}
		},
		"quest.TimerQuest": func() script.Script {
			return script.Script{Hooks: script.Hooks{OnTimer: func(s *script.Script, e script.Timer) string {
				log.record("TimerQuest", e)
				if e.Player != nil {
					s.PlaySound(e.Player, timerSound)
				}
				return ""
			}}}
		},
	}
	list := []script.Listing{{Path: "ai.TimerWolf"}, {Path: "quest.TimerQuest"}}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Timer", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{timerWolfID: script.KindHostile}, list, catalog),
	}, opts...)...)
	return srv, srv.SoleObjectID(t), script.NewSpawner(srv.NpcSpawns, zerolog.Nop())
}

// sounds returns the PlaySound lines among the frames queued since the
// last read.
func sounds(t *testing.T, srv *gameservertest.Server) []string {
	t.Helper()
	var out []string
	for _, line := range sentLines(t, srv) {
		if strings.HasPrefix(line, "S PlaySound") {
			out = append(out, line)
		}
	}
	return out
}

// runTimers runs fn as one invocation of the named timer script on the
// player's queue and returns the handle it ran with.
func runTimers(t *testing.T, srv *gameservertest.Server, objID int32, name string, fn func(s *script.Script, p *script.Player)) (*script.Script, *script.Player) {
	t.Helper()
	var sc *script.Script
	var pl *script.Player
	if !srv.RunScript(t, objID, name, func(s *script.Script, p *script.Player) {
		sc, pl = s, p
		fn(s, p)
	}) {
		t.Fatalf("script %s panicked", name)
	}
	return sc, pl
}

// A timer bound to a player fires on the player's queue and reaches the
// player; it stops when the player leaves the world, for every script,
// while a timer bound to no one keeps going. After the relog the new
// character starts the same timers afresh.
func TestPlayerTimersStopAtRelog(t *testing.T) {
	t.Parallel()
	log := &timerLog{}
	srv, objID, _ := bootTimers(t, log)
	enterWorld(t, srv)

	sc, oldP := runTimers(t, srv, objID, "TimerQuest", func(s *script.Script, p *script.Player) {
		if !s.StartTimer("remind", nil, p, 200*time.Millisecond) || !s.StartTimerAtFixedRate("tick", nil, p, 200*time.Millisecond, 200*time.Millisecond) {
			t.Error("a first start was refused")
		}
		if s.StartTimer("remind", nil, p, time.Second) {
			t.Error("a duplicate start was accepted")
		}
		s.StartTimer("engine", nil, nil, time.Second)
	})
	wolfBehavior, _ := runTimers(t, srv, objID, "TimerWolf", func(s *script.Script, p *script.Player) {
		s.StartTimer("behavior", nil, p, 600*time.Millisecond)
	})
	srv.Advance(t, 200*time.Millisecond)
	p := fmt.Sprint(objID)
	// Timers due together fire in any order on the pool, as in the
	// reference's.
	got := log.take()
	slices.Sort(got)
	if want := []string{"TimerQuest remind npc=none player=" + p, "TimerQuest tick npc=none player=" + p}; !slices.Equal(got, want) {
		t.Fatalf("fired %q, want %q", got, want)
	}
	if got := sounds(t, srv); len(got) != 2 {
		t.Fatalf("sounds = %q, want one per firing", got)
	}

	restart(t, srv)
	if sc.HasTimer("tick", nil, oldP) || wolfBehavior.HasTimer("behavior", nil, oldP) {
		t.Fatal("a timer bound to the departed player is still pending")
	}
	srv.Advance(t, time.Second)
	if got, want := log.take(), []string{"TimerQuest engine npc=none player=none"}; !slices.Equal(got, want) {
		t.Fatalf("after the restart fired %q, want only the timer bound to no one", got)
	}

	enterWorld(t, srv)
	srv.Advance(t, time.Second)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("after the relog fired %q, want nothing", got)
	}
	runTimers(t, srv, objID, "TimerQuest", func(s *script.Script, p *script.Player) {
		if !s.StartTimer("remind", nil, p, 200*time.Millisecond) {
			t.Error("the relogged character's start was refused")
		}
	})
	srv.Advance(t, 200*time.Millisecond)
	if got, want := log.take(), []string{"TimerQuest remind npc=none player=" + p}; !slices.Equal(got, want) {
		t.Fatalf("after the relog fired %q, want %q", got, want)
	}
	if got := sounds(t, srv); len(got) != 1 {
		t.Fatalf("sounds after the relog = %q, want one", got)
	}
}

// A behavior's timers bound to its NPC run on the NPC's queue until the
// NPC decays; a plain script's timer bound to the same NPC runs on the
// engine queue, survives the decay and fires with the gone NPC.
func TestNPCTimersStopAtDecay(t *testing.T) {
	t.Parallel()
	log := &timerLog{}
	srv, objID, spawner := bootTimers(t, log)
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	wolf := spawner.AddSpawn(timerWolfID, script.Loc{X: x + 50, Y: y, Z: z}, false, time.Second)
	if wolf == nil {
		t.Fatal("no wolf spawned")
	}
	w := fmt.Sprint(wolf.ObjectID())

	behavior, _ := runTimers(t, srv, objID, "TimerWolf", func(s *script.Script, _ *script.Player) {
		s.StartTimerAtFixedRate("pulse", wolf, nil, 250*time.Millisecond, 250*time.Millisecond)
		s.StartTimer("late", wolf, nil, 2*time.Second)
	})
	runTimers(t, srv, objID, "TimerQuest", func(s *script.Script, _ *script.Player) {
		s.StartTimer("after", wolf, nil, 2*time.Second)
	})
	srv.AdvanceUntil(t, "the wolf's despawn", wolf.Decayed)
	if behavior.HasTimer("pulse", wolf, nil) || behavior.HasTimer("late", wolf, nil) {
		t.Fatal("a behavior timer of the decayed wolf is still pending")
	}
	// On the wall clock a pulse due with the despawn may beat it.
	pulses := log.take()
	if n := len(pulses); n != 3 && (srv.DrivesClock() || n != 4) {
		t.Fatalf("before the decay fired %q, want three pulses", pulses)
	}
	for _, line := range pulses {
		if line != "TimerWolf pulse npc="+w+" player=none" {
			t.Fatalf("before the decay fired %q, want only pulses", pulses)
		}
	}
	srv.Advance(t, 2*time.Second)
	if got, want := log.take(), []string{"TimerQuest after npc=" + w + " player=none"}; !slices.Equal(got, want) {
		t.Fatalf("after the decay fired %q, want %q", got, want)
	}
}

// Starts of one key race from many goroutines while the key's NPC decays
// and its player leaves: exactly one start wins, and no timer bound to
// either is left once both are gone.
func TestTimerRacesWithDecayAndDetach(t *testing.T) {
	t.Parallel()
	log := &timerLog{}
	srv, objID, spawner := bootTimers(t, log, gameservertest.WithRealPool())
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	wolf := spawner.AddSpawn(timerWolfID, script.Loc{X: x + 50, Y: y, Z: z}, false, 50*time.Millisecond)

	behavior, p := runTimers(t, srv, objID, "TimerWolf", func(*script.Script, *script.Player) {})
	var won atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if behavior.StartTimerAtFixedRate("race", wolf, p, 0, time.Millisecond) {
				won.Add(1)
			}
		})
	}
	restart(t, srv)
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("%d starts of one key won, want 1", won.Load())
	}
	srv.AdvanceUntil(t, "the wolf's despawn", wolf.Decayed)
	srv.Settle(t)
	if behavior.HasTimer("race", wolf, p) {
		t.Fatal("the raced timer outlived its NPC and its player")
	}
	if c, ok := wolfOf(srv, wolf); ok {
		t.Fatalf("wolf %d still in the world", c.ObjectID())
	}
}

// wolfOf returns the world's creature of the wolf's object id.
func wolfOf(srv *gameservertest.Server, wolf *script.NPC) (attackable.Combatant, bool) {
	obj, ok := srv.State.Object(wolf.ObjectID())
	if !ok {
		return nil, false
	}
	c, ok := obj.(attackable.Combatant)
	return c, ok
}
