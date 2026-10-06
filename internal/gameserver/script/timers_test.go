package script

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
	"github.com/rs/zerolog"
)

// timerNPCFake is a live NPC of template id on its own queue.
type timerNPCFake struct {
	combatant
	id      int
	queue   *sim.Queue
	scratch *npc.Scratch
	dead    atomic.Bool
	decayed atomic.Bool
}

func (n *timerNPCFake) NpcID() int            { return n.id }
func (n *timerNPCFake) Queue() *sim.Queue     { return n.queue }
func (n *timerNPCFake) Dead() bool            { return n.dead.Load() }
func (n *timerNPCFake) Decayed() bool         { return n.decayed.Load() }
func (n *timerNPCFake) Scratch() *npc.Scratch { return n.scratch }

// timerPlayerFake is a live player on its own queue.
type timerPlayerFake struct {
	combatant
	queue     *sim.Queue
	detaching atomic.Bool
}

func (p *timerPlayerFake) Queue() *sim.Queue { return p.queue }
func (p *timerPlayerFake) Detaching() bool   { return p.detaching.Load() }

// timerWorld is a registry with its engine queue, two NPCs of template
// timerNPCID and two players, each on its own queue of one inline clock.
type timerWorld struct {
	t      *testing.T
	clock  *sim.Inline
	start  time.Time
	engine *sim.Queue
	r      *Registry
	logs   *logBuffer
	npcs   map[string]*NPC
	fakes  map[string]*timerNPCFake
	pls    map[string]*Player
	plf    map[string]*timerPlayerFake
	names  map[any]string

	mu    sync.Mutex
	lines []string
	// onFire runs inside every hook, after its fire line is recorded.
	onFire func(s *Script, e Timer)
}

const timerNPCID = 7

func newTimerWorld(t *testing.T, list []Listing, catalog func(w *timerWorld) Catalog) *timerWorld {
	t.Helper()
	start := time.UnixMilli(0)
	w := &timerWorld{
		t: t, clock: sim.NewInline(start), start: start, logs: &logBuffer{},
		npcs: map[string]*NPC{}, fakes: map[string]*timerNPCFake{}, pls: map[string]*Player{}, plf: map[string]*timerPlayerFake{}, names: map[any]string{},
	}
	w.engine = w.clock.NewQueue("script-timers")
	for i, name := range []string{"npc1", "npc2"} {
		f := &timerNPCFake{combatant: combatant{objectID: int32(100 + i)}, id: timerNPCID, queue: w.clock.NewQueue(name), scratch: npc.NewScratch()}
		w.fakes[name], w.npcs[name] = f, &NPC{self: f}
		w.names[f.scratch] = name
	}
	for i, name := range []string{"p1", "p2"} {
		f := &timerPlayerFake{combatant: combatant{objectID: int32(200 + i)}, queue: w.clock.NewQueue(name)}
		w.plf[name], w.pls[name] = f, &Player{self: f}
		w.names[f] = name
	}
	w.r = Build(list, catalog(w), RaiseAll(Config{KindOf: allTemplates, Log: zerolog.New(w.logs), Queue: w.engine}))
	return w
}

// at is the clock's time since the world began, in milliseconds.
func (w *timerWorld) at() int64 { return w.clock.Now().Sub(w.start).Milliseconds() }

func (w *timerWorld) record(line string) {
	w.mu.Lock()
	w.lines = append(w.lines, line)
	w.mu.Unlock()
}

func (w *timerWorld) take() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.lines
	w.lines = nil
	return out
}

// recorder is a script whose timer hook records its firings as the timer
// goldens write them.
func (w *timerWorld) recorder(label string, behavior bool) func() Script {
	return func() Script {
		s := Script{Hooks: Hooks{OnTimer: func(s *Script, e Timer) string {
			w.record(fmt.Sprintf("fire %s %s npc=%s player=%s at=%d", label, e.Name, w.npcName(e.NPC), w.playerName(e.Player), w.at()))
			if w.onFire != nil {
				w.onFire(s, e)
			}
			return ""
		}}}
		if behavior {
			s.Behavior, s.NPCs = true, []int32{timerNPCID}
			s.OnAttacked = func(*Script, Attacked) {}
		}
		return s
	}
}

func (w *timerWorld) npcName(n *NPC) string {
	if id := npcIdentity(n); id != nil {
		return w.names[id]
	}
	return "none"
}

func (w *timerWorld) playerName(p *Player) string {
	if id := playerIdentity(p); id != nil {
		return w.names[id]
	}
	return "none"
}

func (w *timerWorld) npc(name string) *NPC {
	if name == "none" {
		return nil
	}
	n, ok := w.npcs[name]
	if !ok {
		w.t.Fatalf("no npc %q", name)
	}
	return n
}

func (w *timerWorld) player(name string) *Player {
	if name == "none" {
		return nil
	}
	p, ok := w.pls[name]
	if !ok {
		w.t.Fatalf("no player %q", name)
	}
	return p
}

// script returns the registered script labelled A or B.
func (w *timerWorld) script(label string) *Script {
	for _, e := range w.r.entries {
		if e.script != nil && e.script.Name == label {
			return e.script
		}
	}
	w.t.Fatalf("no script %s", label)
	return nil
}

// advanceTo moves the clock to ms, firing what comes due up to and at it.
func (w *timerWorld) advanceTo(ms int64) {
	if d := ms - w.at(); d >= 0 {
		w.clock.Advance(time.Duration(d) * time.Millisecond)
	}
}

// keyed splits "k=v" words into a map.
func keyed(words []string) map[string]string {
	out := map[string]string{}
	for _, word := range words {
		if k, v, ok := strings.Cut(word, "="); ok {
			out[k] = v
		}
	}
	return out
}

// timerGoldenTables are the timer goldens, replayed in full.
var timerGoldenTables = []string{"timers.identity", "timers.one_shot", "timers.fixed_rate", "timers.cancel", "timers.liveness"}

// TestTimerGoldens replays the reference's timer rules: identity and
// duplicate refusal, one-shot removal before the hook, the fixed-rate
// grid, the cancel variants and the absence of a liveness check. Every
// start and cancel line runs at the time of the line before it; every fire
// line is what the hooks recorded. A start with a null name has no Go
// form (a Go string is never null) and is skipped.
func TestTimerGoldens(t *testing.T) {
	for _, table := range timerGoldenTables {
		t.Run(table, func(t *testing.T) {
			scriptcontract.Run(t, table, func(t *testing.T, row scriptcontract.Row) {
				replayTimerRow(t, row)
			})
		})
	}
}

func replayTimerRow(t *testing.T, row scriptcontract.Row) {
	w := newTimerWorld(t, listOf("quest.A", "quest.B"), func(w *timerWorld) Catalog {
		return Catalog{"quest.A": w.recorder("A", false), "quest.B": w.recorder("B", false)}
	})
	// pending lines are the expected lines after the fire being replayed:
	// its hook's actions.
	var pending []string
	w.onFire = func(s *Script, e Timer) {
		for len(pending) > 0 && strings.HasPrefix(pending[0], "  ") {
			line := strings.TrimSpace(pending[0])
			pending = pending[1:]
			switch {
			case strings.HasPrefix(line, "pending-in-hook="):
				w.record("  pending-in-hook=" + strconv.FormatBool(s.HasTimer(e.Name, e.NPC, e.Player)))
			case strings.HasPrefix(line, "restart "):
				name := strings.Fields(line)[1]
				ok := s.StartTimer(name, e.NPC, e.Player, time.Second)
				w.record(fmt.Sprintf("  restart %s -> %v", name, ok))
			default:
				t.Fatalf("unknown hook action %q", line)
			}
		}
	}
	var want []string
	lines := row.Lines
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		words := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "  "):
			// Run by the hook of the fire line above.
			continue
		case words[0] == "start" && words[2] == "null":
			continue
		case words[0] == "start":
			want = append(want, line)
			kv := keyed(words[3:])
			s, n, p := w.script(words[1]), w.npc(kv["npc"]), w.player(kv["player"])
			var ok bool
			var detail string
			if d, one := kv["delay"]; one {
				ms, _ := strconv.Atoi(d)
				ok = s.StartTimer(words[2], n, p, time.Duration(ms)*time.Millisecond)
				detail = "delay=" + d
			} else {
				initial, _ := strconv.Atoi(kv["initial"])
				period, _ := strconv.Atoi(kv["period"])
				ok = s.StartTimerAtFixedRate(words[2], n, p, time.Duration(initial)*time.Millisecond, time.Duration(period)*time.Millisecond)
				detail = fmt.Sprintf("initial=%s period=%s", kv["initial"], kv["period"])
			}
			w.record(fmt.Sprintf("start %s %s npc=%s player=%s %s -> %v", words[1], words[2], kv["npc"], kv["player"], detail, ok))
		case words[0] == "fire":
			want = append(want, line)
			j := i + 1
			for j < len(lines) && strings.HasPrefix(lines[j], "  ") {
				want = append(want, lines[j])
				j++
			}
			pending = lines[i+1 : j]
			at, _ := strconv.ParseInt(keyed(words)["at"], 10, 64)
			w.advanceTo(at)
		case words[0] == "cancel":
			want = append(want, line)
			w.cancel(words[1:])
			w.record(line)
		case words[0] == "delete":
			// The NPC is deleted and the player leaves the world: the
			// decay removal runs, which stops no plain script's timer.
			want = append(want, line)
			f := w.fakes[strings.TrimSuffix(words[1], ";")]
			f.dead.Store(true)
			f.decayed.Store(true)
			w.r.timers.npcDecayed(f.scratch)
			w.record(line)
		default:
			t.Fatalf("unknown golden line %q", line)
		}
	}
	w.advanceTo(w.at() + 10_000)
	if got := w.take(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("timer lines:\n%s\nreference:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// cancel runs one cancel line of the goldens on script A.
func (w *timerWorld) cancel(words []string) {
	s := w.script("A")
	switch words[0] {
	case "all":
		s.CancelTimers()
	case "exact":
		s.CancelTimer(words[1], w.npc(words[2]), w.player(words[3]))
	default:
		if kv := keyed(words[1:]); len(kv) > 0 {
			// "<name> npc=<npc> player=<player>": the exact cancel.
			s.CancelTimer(words[0], w.npc(kv["npc"]), w.player(kv["player"]))
			return
		}
		var filters []TimerFilter
		for i := 0; i+1 < len(words); i += 2 {
			switch words[i] {
			case "name":
				filters = append(filters, TimerName(words[i+1]))
			case "npc":
				filters = append(filters, TimerNPC(w.npc(words[i+1])))
			case "player":
				filters = append(filters, TimerPlayer(w.player(words[i+1])))
			default:
				w.t.Fatalf("unknown cancel %q", words)
			}
		}
		s.CancelTimers(filters...)
	}
}
