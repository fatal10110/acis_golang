package task

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// ---- from decay_test.go ----
type decayFakeActor struct {
	id int32
}

func (a *decayFakeActor) ObjectID() int32 { return a.id }

type decayFakeEffects struct {
	mu     sync.Mutex
	events []string
}

type decayNoopEffects struct{}

func (decayNoopEffects) Decay(DecayActor) {}

func (e *decayFakeEffects) Decay(actor DecayActor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, fmt.Sprintf("decay %d", actor.ObjectID()))
}

func (e *decayFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestNewDecayRejectsNilEffects(t *testing.T) {
	if _, err := NewDecay(nil, nil); err == nil {
		t.Fatal("NewDecay() error = nil, want error for nil effects")
	}
}

func TestDecayAddThenTickFiresAfterDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, 7*time.Second)
	if !decay.Tracked(actor) {
		t.Fatal("actor should be tracked after Add")
	}

	now = now.Add(6 * time.Second)
	decay.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	decay.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"decay 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if decay.Tracked(actor) {
		t.Fatal("actor should be removed after decay fires")
	}
}

func TestDecayTickAllocationIsFlat(t *testing.T) {
	now := time.UnixMilli(0)
	decay, err := NewDecay(decayNoopEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	actors := make([]*decayFakeActor, 128)
	for i := 0; i < 128; i++ {
		actors[i] = &decayFakeActor{id: int32(i + 1)}
	}
	tick := func() {
		for _, actor := range actors {
			decay.Add(actor, -time.Second)
		}
		decay.Tick()
		testLoop.Run()
	}
	tick()
	for i, entry := range decay.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after Tick", i)
		}
	}

	// Each due actor costs the one task posted to its queue. The sweep
	// itself allocates nothing; the few extra allow for the test loop
	// growing its task slice. The budget describes the production build.
	if simdebugBuild {
		return
	}
	if allocs := testing.AllocsPerRun(100, tick); allocs > float64(len(actors)+4) {
		t.Fatalf("AllocsPerRun(128 actors) = %v, want <= %d", allocs, len(actors)+4)
	}
}

type decayPanicEffects struct{}

func (decayPanicEffects) Decay(DecayActor) { panic("boom") }

func TestDecayTickClearsScratchOnPanic(t *testing.T) {
	now := time.UnixMilli(0)
	decay, err := NewDecay(decayPanicEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	decay.Add(&decayFakeActor{id: 1}, -time.Second)

	func() {
		defer func() { recover() }()
		decay.Tick()
		testLoop.Run()
	}()

	for i, entry := range decay.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after panicking Tick", i)
		}
	}
	if decay.ticking.Load() {
		t.Fatal("ticking guard left set after panicking Tick")
	}
}

// TestDecayTickReturnsErrorOnReentrantCall covers a Tick that starts while
// another is still in flight: it does nothing and reports it.
func TestDecayTickReturnsErrorOnReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	decay.Add(&decayFakeActor{id: 1}, -time.Second)
	decay.ticking.Store(true) // another Tick is in flight

	if err := decay.Tick(); !errors.Is(err, ErrReentrantTick) {
		t.Fatalf("reentrant Tick() error = %v, want ErrReentrantTick", err)
	}
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("reentrant Tick events = %v, want none", got)
	}
}

func TestDecayTickLogsReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	decay, err := NewDecay(&decayFakeEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	var buf bytes.Buffer
	decay.log = zerolog.New(&buf)
	decay.ticking.Store(true) // another Tick is in flight

	decay.Tick()
	testLoop.Run()

	if !strings.Contains(buf.String(), "Decay.Tick") || !strings.Contains(buf.String(), ErrReentrantTick.Error()) {
		t.Fatalf("reentrant Tick call was not logged, got %q", buf.String())
	}
}

func BenchmarkDecayTickManyActors(b *testing.B) {
	now := time.UnixMilli(0)
	decay, err := NewDecay(decayNoopEffects{}, func() time.Time { return now })
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 4096; i++ {
		decay.Add(&decayFakeActor{id: int32(i + 1)}, time.Hour)
	}
	decay.Tick()
	testLoop.Run()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decay.Tick()
		testLoop.Run()
	}
}

func TestDecayDeadlineReportsTrackedDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	if _, ok := decay.Deadline(actor); ok {
		t.Fatal("Deadline() ok = true before Add, want false")
	}

	decay.Add(actor, 7*time.Second)
	if got, ok := decay.Deadline(actor); !ok || !got.Equal(now.Add(7*time.Second)) {
		t.Fatalf("Deadline() = %v, %v; want %v, true", got, ok, now.Add(7*time.Second))
	}

	decay.Cancel(actor)
	if _, ok := decay.Deadline(actor); ok {
		t.Fatal("Deadline() ok = true after Cancel, want false")
	}
}

func TestDecayCancelStopsPendingDecay(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, time.Second)

	if !decay.Cancel(actor) {
		t.Fatal("Cancel() = false, want true for tracked actor")
	}
	if decay.Cancel(actor) {
		t.Fatal("Cancel() = true, want false for already-removed actor")
	}

	now = now.Add(time.Hour)
	decay.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after cancel = %v, want none", got)
	}
}

func TestDecayAddReplacesExistingDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, time.Second)
	decay.Add(actor, 10*time.Second)

	now = now.Add(time.Second)
	decay.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before replaced deadline = %v, want none", got)
	}

	now = now.Add(9 * time.Second)
	decay.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"decay 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at replaced deadline = %v, want %v", got, want)
	}
}

func TestDecayConcurrentAddAndTick(t *testing.T) {
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, nil)
	actors := make([]*decayFakeActor, 100)
	for i := range actors {
		actors[i] = &decayFakeActor{id: int32(i)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for _, actor := range actors {
			decay.Add(actor, 0)
		}
	}()
	go func() {
		defer wg.Done()
		for _, actor := range actors {
			decay.Cancel(actor)
		}
	}()
	go func() {
		defer wg.Done()
		for range actors {
			decay.Tick()
			testLoop.Run()
		}
	}()
	wg.Wait()
}

type decayFakeSummon struct {
	id     int32
	linked bool
}

func (a *decayFakeSummon) ObjectID() int32 { return a.id }

func (a *decayFakeSummon) OwnerStillLinked() bool { return a.linked }

func TestDecayOrphanedSummonCancelledBeforeDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	summon := &decayFakeSummon{id: 200, linked: false}
	decay.Add(summon, time.Hour)
	if !decay.Tracked(summon) {
		t.Fatal("orphaned summon should be tracked immediately after Add")
	}

	decay.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick cancelled orphaned summon = %v, want none", got)
	}
	if decay.Tracked(summon) {
		t.Fatal("orphaned summon should be untracked after Tick")
	}
}

func TestDecayLinkedSummonDecaysAtDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	summon := &decayFakeSummon{id: 201, linked: true}
	decay.Add(summon, time.Second)

	now = now.Add(time.Second)
	decay.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"decay 201"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if decay.Tracked(summon) {
		t.Fatal("linked summon should be removed after decay fires")
	}
}

func TestDecayOrphanedSummonCancelledEvenWhenDue(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	summon := &decayFakeSummon{id: 202, linked: false}
	decay.Add(summon, -time.Second)

	decay.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("due orphaned summon = %v, want cancel without decay", got)
	}
	if decay.Tracked(summon) {
		t.Fatal("due orphaned summon should be untracked after Tick")
	}
}

func TestDecayNonSummonActorsUnaffectedByLinkageCheck(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	npc := &decayFakeActor{id: 300}
	decay.Add(npc, time.Second)

	decay.Tick()
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before NPC deadline = %v, want none", got)
	}
	if !decay.Tracked(npc) {
		t.Fatal("NPC should remain tracked before deadline")
	}

	now = now.Add(time.Second)
	decay.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"decay 300"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at NPC deadline = %v, want %v", got, want)
	}
}

func TestShadowItems_TrackDecaysManaEachTick(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, err := NewShadowItems(effects)
	if err != nil {
		t.Fatalf("NewShadowItems() error = %v", err)
	}

	tmpl := &item.Template{Duration: 5} // 300 seconds of mana
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}

	s.Track(10, inst, tmpl)
	if !s.Tracked(inst) {
		t.Fatalf("Track() should start tracking a shadow item")
	}
	if inst.ManaLeft != 300 {
		t.Fatalf("first Track() must not cost extra mana, ManaLeft = %d, want 300", inst.ManaLeft)
	}

	s.Tick()
	if inst.ManaLeft != 299 {
		t.Errorf("ManaLeft after one tick = %d, want 299", inst.ManaLeft)
	}
}

func (*decayFakeActor) Queue() *sim.Queue { return testQueue }

func (*decayFakeSummon) Queue() *sim.Queue { return testQueue }

// decayMovingSummon is a summon whose queue moves the first time it is read,
// the way a corpse moves to its own queue while its owner's queue closes.
type decayMovingSummon struct {
	id       int32
	mu       sync.Mutex
	current  *sim.Queue
	moveTo   *sim.Queue
	moveOnce bool
}

func (a *decayMovingSummon) ObjectID() int32 { return a.id }

func (a *decayMovingSummon) OwnerStillLinked() bool { return true }

func (a *decayMovingSummon) Queue() *sim.Queue {
	a.mu.Lock()
	defer a.mu.Unlock()
	q := a.current
	if !a.moveOnce {
		a.moveOnce = true
		a.current = a.moveTo
		q.Close()
	}
	return q
}

// TestDecayFollowsCorpseToItsNewQueue reads a due corpse's queue just before
// its owner's detach moves the corpse to a queue of its own and closes the
// owner's. The decay the owner's queue refuses still runs, on the new queue.
func TestDecayFollowsCorpseToItsNewQueue(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	corpse := &decayMovingSummon{id: 210, current: testLoop.NewQueue("owner"), moveTo: testLoop.NewQueue("corpse")}
	decay.Add(corpse, time.Second)

	now = now.Add(time.Second)
	decay.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"decay 210"}; !slices.Equal(got, want) {
		t.Fatalf("decay after the corpse changed queues = %v, want %v", got, want)
	}
}
