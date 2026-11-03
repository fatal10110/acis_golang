package script

import (
	"bytes"
	"runtime"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// departureProbe writes a fresh character through the registry's count of
// running invocations, then makes it leave the world on another goroutine
// while the calling invocation is still running. held reports whether the
// departure stopped to wait for that invocation.
type departureProbe struct {
	r    *Registry
	held bool
	done chan struct{}
}

func (p *departureProbe) run() {
	c := &player.Character{}
	c.Configure(player.Runtime{ScriptRuns: p.r.ScriptRuns()})
	c.WhileAttached(func() {})
	p.done = make(chan struct{})
	go func() {
		c.MarkDetaching()
		close(p.done)
	}()
	for {
		select {
		case <-p.done:
			return
		default:
		}
		if departureParked() {
			// Returning ends the invocation, which lets the departure go.
			p.held = true
			return
		}
		runtime.Gosched()
	}
}

// departureParked reports whether a departure is parked waiting for script
// invocations to end.
func departureParked() bool {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	for _, g := range bytes.Split(buf, []byte("\n\n")) {
		header, _, _ := bytes.Cut(g, []byte("\n"))
		if bytes.Contains(header, []byte("[sync.Cond.Wait")) && bytes.Contains(g, []byte("(*ScriptRuns).awaitEnd(")) {
			return true
		}
	}
	return false
}

// TestScheduleStartDoesNotHoldDepartures: a start hook, which may hold the
// database for long, does not hold the departure of a player written while
// it runs; a hook of any other kind does, the probe's own control.
func TestScheduleStartDoesNotHoldDepartures(t *testing.T) {
	in := sim.NewInline(time.Date(2026, 10, 5, 23, 59, 0, 0, time.UTC))
	probe := &departureProbe{}
	started := false
	r, _ := buildScheduled(t,
		[]Listing{{Path: "task.Daily", Schedule: "DAILY", Start: "00:00:00"}},
		Catalog{"task.Daily": func() Script {
			return Script{Hooks: Hooks{OnStart: func(*Script, Start) {
				started = true
				probe.run()
			}}}
		}})
	probe.r = r
	sch := StartSchedule(r, in.NewQueue("schedule"), recordingServer{})
	defer sch.Stop()

	in.Advance(time.Hour)
	if !started {
		t.Fatal("the start hook did not run")
	}
	<-probe.done
	if probe.held {
		t.Fatal("a departure waited for a running start hook")
	}

	control := &departureProbe{r: r}
	r.run(&Script{path: "control"}, hookEvent, control.run)
	<-control.done
	if !control.held {
		t.Fatal("a departure did not wait for a running script invocation that wrote the player")
	}
}
