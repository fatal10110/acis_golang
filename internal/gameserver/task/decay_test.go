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

	"github.com/rs/zerolog"
)

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
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	decay.Tick()
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
	}
	tick()
	for i, entry := range decay.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after Tick", i)
		}
	}

	if allocs := testing.AllocsPerRun(100, tick); allocs != 0 {
		t.Fatalf("AllocsPerRun(128 actors) = %v, want 0", allocs)
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

type decayReentrantEffects struct {
	decay    *Decay
	innerErr error
}

func (e *decayReentrantEffects) Decay(DecayActor) {
	e.innerErr = e.decay.Tick()
}

func TestDecayTickReturnsErrorOnReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayReentrantEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	effects.decay = decay
	decay.Add(&decayFakeActor{id: 1}, -time.Second)

	if err := decay.Tick(); err != nil {
		t.Fatalf("outer Tick() error = %v, want nil", err)
	}

	if !errors.Is(effects.innerErr, ErrReentrantTick) {
		t.Fatalf("reentrant Tick() error = %v, want ErrReentrantTick", effects.innerErr)
	}
	if decay.ticking.Load() {
		t.Fatal("ticking guard left set after outer Tick returned")
	}
}

func TestDecayTickLogsReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayReentrantEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	effects.decay = decay
	var buf bytes.Buffer
	decay.log = zerolog.New(&buf)
	decay.Add(&decayFakeActor{id: 1}, -time.Second)

	decay.Tick()

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

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decay.Tick()
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
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before replaced deadline = %v, want none", got)
	}

	now = now.Add(9 * time.Second)
	decay.Tick()
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
		}
	}()
	wg.Wait()
}
