package summon

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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

func (g *stallingGate) Test(stat.Actor) bool {
	if g.armed.CompareAndSwap(true, false) {
		close(g.entered)
		<-g.release
	}
	return true
}

// TestSummonMoveSpeedRefreshKeepsLatestStats interleaves two stat
// refreshes: the first reads the stats and stalls before handing its speed
// to the movement, while a slow lands and refreshes. The movement must end
// at the slowed speed, not the unslowed one the first refresh read.
func TestSummonMoveSpeedRefreshKeepsLatestStats(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})
	if err := a.InitMovement(location.Location{}, 120, openGeo{}); err != nil {
		t.Fatalf("InitMovement: %v", err)
	}
	full := a.MoveSpeed(120)

	gate := &stallingGate{entered: make(chan struct{}), release: make(chan struct{})}
	gate.armed.Store(true)
	firstDone := make(chan struct{})
	go func() {
		a.AddStatFuncs([]effect.Mod{{Stat: stat.RunSpeed, Op: effect.OpMul, Value: 1, Cond: gate}})
		close(firstDone)
	}()
	<-gate.entered

	slowDone := make(chan struct{})
	go func() {
		a.AddStatFuncs([]effect.Mod{{Stat: stat.RunSpeed, Op: effect.OpMul, Value: 0.5}})
		close(slowDone)
	}()
	// Without serialization the slow's refresh finishes while the first is
	// stalled; with it, the slow's refresh waits its turn.
	select {
	case <-slowDone:
	case <-time.After(200 * time.Millisecond):
	}
	close(gate.release)
	<-firstDone
	<-slowDone

	want := float64(float32(full / 2))
	if got := a.Move().Speed(); got != want {
		t.Fatalf("Move().Speed() = %v, want the slowed speed %v (unslowed %v)", got, want, full)
	}
}
