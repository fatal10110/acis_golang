package effect

import (
	"sync"
	"sync/atomic"
	"testing"
)

// iconGate is a StatOwner whose next icon refresh, once armed, signals
// entered and blocks until release closes: it holds a Remove between its
// icon refresh and the removed head's exit hook.
type iconGate struct {
	*modLedger
	armed   atomic.Bool
	fired   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newIconGate() *iconGate {
	return &iconGate{
		modLedger: newModLedger(),
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
}

func (o *iconGate) UpdateEffectIcons() {
	if !o.armed.Load() || !o.fired.CompareAndSwap(false, true) {
		return
	}
	close(o.entered)
	<-o.release
}

// TestDisplacedHeadExitRunsBeforeLaterHeadStart lands a third stack order
// while the exit hook of the head the second one displaced is still running
// on another goroutine. The third one's start waits for that exit hook, so
// the flag it sets survives.
func TestDisplacedHeadExitRunsBeforeLaterHeadStart(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  Env
	}{
		{name: "lesser cancelled"},
		{name: "lesser kept", env: Env{KeepLesser: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			list := newTestList(newModLedger(), WithEnv(tc.env))
			flag := newSharedFlag()
			low := newEffect("low", 1, "petrification", 1, true)
			mid := newEffect("mid", 2, "petrification", 2, true)
			high := newEffect("high", 3, "petrification", 3, true)
			for _, e := range []*Effect{low, mid, high} {
				flag.hook(e)
			}
			list.Add(low)

			entered, release := make(chan struct{}), make(chan struct{})
			exit := low.OnExit
			low.OnExit = func(e *Effect) {
				close(entered)
				<-release
				exit(e)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				list.Add(mid)
			}()
			<-entered
			list.Add(high)
			close(release)
			<-done

			if !high.InUse() || mid.InUse() || low.InUse() {
				t.Fatalf("low/mid/high in use = %v/%v/%v, want high alone active", low.InUse(), mid.InUse(), high.InUse())
			}
			on, starts, exits := flag.state(high)
			if starts != 1 || exits != 0 {
				t.Errorf("high started %d and exited %d times, want one start", starts, exits)
			}
			if _, starts, exits = flag.state(low); starts != 1 || exits != 1 {
				t.Errorf("low started %d and exited %d times, want 1 and 1", starts, exits)
			}
			if !on {
				t.Error("flag cleared while high heads its group, want low's exit hook to run before high's start")
			}
		})
	}
}

// TestRemovedHeadExitRunsBeforeLaterHeadStart lands a higher stack order
// while a Remove of the head waits on its icon refresh, before it ran the
// removed head's exit hook. The newcomer runs that exit hook ahead of its
// own start, and the Remove does not run it again.
func TestRemovedHeadExitRunsBeforeLaterHeadStart(t *testing.T) {
	owner := newIconGate()
	list := newTestList(owner)
	flag := newSharedFlag()
	low := newEffect("low", 1, "petrification", 1, true)
	high := newEffect("high", 2, "petrification", 2, true)
	flag.hook(low)
	flag.hook(high)
	list.Add(low)

	owner.armed.Store(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		list.Remove(low)
	}()
	<-owner.entered
	list.Add(high)
	close(owner.release)
	<-done

	if !high.InUse() {
		t.Fatal("high not in use, want it heading its group")
	}
	on, starts, exits := flag.state(low)
	if starts != 1 || exits != 1 {
		t.Errorf("low started %d and exited %d times, want 1 and 1", starts, exits)
	}
	if !on {
		t.Error("flag cleared while high heads its group, want low's exit hook to run before high's start")
	}
}

// TestRacingStackChangesLeaveHeadStateSet races three changes to one stack
// group whose lowest order heads it: two higher adds and the head's
// removal. Whatever the interleaving, the highest order ends up active with
// its flag set, and no effect exits more often than it started.
func TestRacingStackChangesLeaveHeadStateSet(t *testing.T) {
	for _, env := range []Env{{}, {KeepLesser: true}} {
		for i := range 300 {
			list := newTestList(newModLedger(), WithEnv(env))
			flag := newSharedFlag()
			low := newEffect("low", 1, "petrification", 1, true)
			mid := newEffect("mid", 2, "petrification", 2, true)
			high := newEffect("high", 3, "petrification", 3, true)
			for _, e := range []*Effect{low, mid, high} {
				flag.hook(e)
			}
			list.Add(low)

			var ready, wg sync.WaitGroup
			gate := make(chan struct{})
			for _, change := range []func(){
				func() { list.Add(mid) },
				func() { list.Add(high) },
				func() { list.Remove(low) },
			} {
				ready.Add(1)
				wg.Add(1)
				go func() {
					defer wg.Done()
					ready.Done()
					<-gate
					change()
				}()
			}
			ready.Wait()
			close(gate)
			wg.Wait()

			if !high.InUse() || mid.InUse() {
				t.Fatalf("keep lesser %v, iteration %d: mid/high in use = %v/%v, want high alone active", env.KeepLesser, i, mid.InUse(), high.InUse())
			}
			on, starts, exits := flag.state(high)
			if starts != 1 || exits != 0 || !on {
				t.Fatalf("keep lesser %v, iteration %d: high started %d, exited %d, flag %v; want one start and the flag set", env.KeepLesser, i, starts, exits, on)
			}
			for _, e := range []*Effect{low, mid} {
				if _, starts, exits := flag.state(e); exits > starts || starts > 1 {
					t.Fatalf("keep lesser %v, iteration %d: %s started %d and exited %d times", env.KeepLesser, i, e.Template.Name, starts, exits)
				}
			}
		}
	}
}
