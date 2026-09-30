package npc

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// stallingGate is an always-true stat func condition whose first test once
// armed blocks until released, holding a speed refresh mid-compute.
type stallingGate struct {
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newStallingGate() *stallingGate {
	return &stallingGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *stallingGate) Test(stat.Actor) bool {
	if g.armed.CompareAndSwap(true, false) {
		close(g.entered)
		<-g.release
	}
	return true
}

// TestHostileMoveSpeedRefreshKeepsLatestStance interleaves a stat refresh
// with a stance change: a slow lands while the monster walks, and before
// that refresh hands its walk-based speed to the movement, the monster
// switches to run. The movement must end at the debuffed run speed, not the
// debuffed walk speed the slower refresh read.
func TestHostileMoveSpeedRefreshKeepsLatestStance(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{
		ID: 9001, Type: "Monster", CanMove: true,
		RunSpeed: 120, WalkSpeed: 60, DEX: 30,
	})
	h.SetRunning(false)

	gate := newStallingGate()
	gate.armed.Store(true)
	slowDone := make(chan struct{})
	go func() {
		h.AddStatFuncs([]effect.Mod{{Stat: stat.RunSpeed, Op: effect.OpMul, Value: 0.5, Cond: gate}})
		close(slowDone)
	}()
	<-gate.entered

	runDone := make(chan struct{})
	go func() {
		h.SetRunning(true)
		close(runDone)
	}()
	// Without serialization the stance refresh finishes while the slow's
	// refresh is stalled; with it, the stance refresh waits its turn.
	select {
	case <-runDone:
	case <-time.After(200 * time.Millisecond):
	}
	close(gate.release)
	<-slowDone
	<-runDone

	// 120 through DEX 30's 1.1 and the 0.5 slow.
	if got, want := h.Move().Speed(), 66.0; got != want {
		t.Fatalf("Move().Speed() = %v, want the debuffed run speed %v", got, want)
	}
}
