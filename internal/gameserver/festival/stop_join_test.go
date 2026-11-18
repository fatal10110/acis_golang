package festival

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// observedTimer is an armed timer whose Stop closes stopped, so a test
// learns when Manager.Stop has latched.
type observedTimer struct {
	once    sync.Once
	stopped chan struct{}
}

func (t *observedTimer) Stop() bool {
	t.once.Do(func() { close(t.stopped) })
	return true
}

// heldSettleWorld keeps Settle's continuation for the test to release and
// records the shouts.
type heldSettleWorld struct {
	*movesWorld
	settled func()
	shouts  []string
}

func (w *heldSettleWorld) Settle(_ []int32, then func()) { w.settled = then }
func (w *heldSettleWorld) Shout(_, text string)          { w.shouts = append(w.shouts, text) }

// A Stop arriving while a festival's opening still waits on its parties'
// queues waits for the step to end: the opening shout still goes out
// before Stop returns, and nothing follows it.
func TestStopJoinsTheStepUnderWay(t *testing.T) {
	h := newScheduleHarness(t)
	w := &heldSettleWorld{movesWorld: newMovesWorld()}
	h.m.SetWorld(w)
	timer := &observedTimer{stopped: make(chan struct{})}
	h.timer = timer
	h.m.Start()
	h.signUp(sevensigns.Dawn, 0, party)
	h.fire(t) // the first cycle
	h.fire(t) // the signup ends: the festival opens
	if w.settled == nil {
		t.Fatal("the opening did not wait on its parties")
	}

	returned := make(chan struct{})
	go func() {
		h.m.Stop(context.Background())
		close(returned)
	}()
	<-timer.stopped
	select {
	case <-returned:
		t.Fatal("Stop returned with the opening still under way")
	default:
	}
	w.settled()
	<-returned

	if !slices.Equal(w.shouts, []string{startingText}) {
		t.Fatalf("shouts = %q, want the opening's only", w.shouts)
	}
}

// A Stop whose context ends while a festival's opening still waits on its
// parties' queues returns without the step.
func TestStopGivesUpOnTheStepWhenItsContextEnds(t *testing.T) {
	h := newScheduleHarness(t)
	w := &heldSettleWorld{movesWorld: newMovesWorld()}
	h.m.SetWorld(w)
	timer := &observedTimer{stopped: make(chan struct{})}
	h.timer = timer
	h.m.Start()
	h.signUp(sevensigns.Dawn, 0, party)
	h.fire(t) // the first cycle
	h.fire(t) // the signup ends: the festival opens
	if w.settled == nil {
		t.Fatal("the opening did not wait on its parties")
	}

	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})
	go func() {
		h.m.Stop(ctx)
		close(returned)
	}()
	<-timer.stopped
	select {
	case <-returned:
		t.Fatal("Stop returned with the opening still under way")
	default:
	}
	cancel()
	<-returned
	if len(w.shouts) != 0 {
		t.Fatalf("shouts = %q, want none before the step ends", w.shouts)
	}
}
