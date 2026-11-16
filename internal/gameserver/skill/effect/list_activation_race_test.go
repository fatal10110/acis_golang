package effect

import (
	"sync"
	"testing"
)

// sharedFlag is owner state that every effect of one kind sets on start and
// clears on exit, as petrification does with invulnerability: the last hook
// to run wins. It counts the starts and exits of each effect.
type sharedFlag struct {
	mu     sync.Mutex
	on     bool
	starts map[*Effect]int
	exits  map[*Effect]int
}

func newSharedFlag() *sharedFlag {
	return &sharedFlag{starts: make(map[*Effect]int), exits: make(map[*Effect]int)}
}

// hook makes e set the flag on start and clear it on exit.
func (f *sharedFlag) hook(e *Effect) {
	e.OnStart = func(e *Effect) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.on = true
		f.starts[e]++
		return true
	}
	e.OnExit = func(e *Effect) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.on = false
		f.exits[e]++
	}
}

func (f *sharedFlag) state(e *Effect) (on bool, starts, exits int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on, f.starts[e], f.exits[e]
}

// TestStaleActivationUndoKeepsDisplacingHeadState lands a higher stack order
// of the same group from another goroutine while the lower one's start hook
// runs. The lower activation is stale by the time it applies, and its undo
// must not clear the flag the higher one set: the higher one starts after
// that undo.
func TestStaleActivationUndoKeepsDisplacingHeadState(t *testing.T) {
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
			lower := newEffect("lower", 1, "petrification", 1, true)
			higher := newEffect("higher", 2, "petrification", 2, true)
			flag.hook(lower)
			flag.hook(higher)
			start := lower.OnStart
			lower.OnStart = func(e *Effect) bool {
				ok := start(e)
				done := make(chan struct{})
				go func() {
					defer close(done)
					list.Add(higher)
				}()
				<-done
				return ok
			}

			list.Add(lower)

			if lower.InUse() || !higher.InUse() {
				t.Fatalf("lower/higher in use = %v/%v, want the higher one alone active", lower.InUse(), higher.InUse())
			}
			on, starts, exits := flag.state(lower)
			if starts != 1 || exits != 1 {
				t.Errorf("lower started %d and exited %d times, want its start undone once", starts, exits)
			}
			_, starts, exits = flag.state(higher)
			if starts != 1 || exits != 0 {
				t.Errorf("higher started %d and exited %d times, want one start", starts, exits)
			}
			if !on {
				t.Error("flag cleared while the higher effect heads its group, want the stale undo to run before its start")
			}
		})
	}
}

// TestDoublePromotionStartsOnce promotes the same stack member twice: while
// its first activation's start hook runs, another goroutine lands a higher
// order over it and removes that again, which queues a second promotion.
// The member starts once, and its removal exits it once.
func TestDoublePromotionStartsOnce(t *testing.T) {
	list := newTestList(newModLedger(), WithEnv(Env{KeepLesser: true}))
	flag := newSharedFlag()
	member := newEffect("member", 1, "petrification", 1, true)
	head := newEffect("head", 3, "petrification", 3, true)
	middle := newEffect("middle", 2, "petrification", 2, true)
	list.Add(member)
	list.Add(head)
	flag.hook(member)
	start := member.OnStart
	armed := true
	member.OnStart = func(e *Effect) bool {
		ok := start(e)
		if armed {
			armed = false
			done := make(chan struct{})
			go func() {
				defer close(done)
				list.Add(middle)
				list.Remove(middle)
			}()
			<-done
		}
		return ok
	}

	list.Remove(head)

	if !member.InUse() {
		t.Fatal("member not in use after its promotions, want it active")
	}
	on, starts, exits := flag.state(member)
	if starts != 1 || exits != 0 || !on {
		t.Fatalf("member started %d and exited %d times, flag %v; want one start and the flag set", starts, exits, on)
	}
	list.Remove(member)
	if _, starts, exits = flag.state(member); starts != 1 || exits != 1 {
		t.Fatalf("member started %d and exited %d times after its removal, want 1 and 1", starts, exits)
	}
}

// TestRacingStackAddsLeaveHeadStateSet races a lower and a higher stack
// order of one group onto a fresh list. Whatever the interleaving, the
// higher one ends up heading the group with its flag set, and every start
// that ran is matched by at most one exit.
func TestRacingStackAddsLeaveHeadStateSet(t *testing.T) {
	for i := range 500 {
		list := newTestList(newModLedger())
		flag := newSharedFlag()
		lower := newEffect("lower", 1, "petrification", 1, true)
		higher := newEffect("higher", 2, "petrification", 2, true)
		flag.hook(lower)
		flag.hook(higher)

		var ready, wg sync.WaitGroup
		gate := make(chan struct{})
		for _, e := range []*Effect{lower, higher} {
			ready.Add(1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				ready.Done()
				<-gate
				list.Add(e)
			}()
		}
		ready.Wait()
		close(gate)
		wg.Wait()

		if !higher.InUse() || lower.InUse() {
			t.Fatalf("iteration %d: lower/higher in use = %v/%v, want the higher one alone active", i, lower.InUse(), higher.InUse())
		}
		on, starts, exits := flag.state(lower)
		if exits > starts || starts > 1 {
			t.Fatalf("iteration %d: lower started %d and exited %d times", i, starts, exits)
		}
		_, starts, exits = flag.state(higher)
		if starts != 1 || exits != 0 {
			t.Fatalf("iteration %d: higher started %d and exited %d times, want one start", i, starts, exits)
		}
		if !on {
			t.Fatalf("iteration %d: flag cleared while the higher effect heads its group", i)
		}
	}
}
