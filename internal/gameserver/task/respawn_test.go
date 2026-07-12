package task

import (
	"slices"
	"sync"
	"testing"
	"time"
)

type respawnFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *respawnFakeEffects) Respawn(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, key)
}

func (e *respawnFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestNewRespawnRejectsNilEffects(t *testing.T) {
	if _, err := NewRespawn(nil, nil); err == nil {
		t.Fatal("NewRespawn() error = nil, want error for nil effects")
	}
}

func TestRespawnAddThenTickFiresAfterDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, err := NewRespawn(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewRespawn() error = %v", err)
	}

	r.Add("slot-1", now.Add(7*time.Second))
	if !r.Tracked("slot-1") {
		t.Fatal("slot should be tracked after Add")
	}

	now = now.Add(6 * time.Second)
	r.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	r.Tick()
	if got, want := effects.take(), []string{"slot-1"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if r.Tracked("slot-1") {
		t.Fatal("slot should be removed after respawn fires")
	}
}

func TestRespawnAddWithPastDeadlineFiresOnNextTick(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, func() time.Time { return now })

	r.Add("slot-1", now.Add(-time.Minute))
	r.Tick()
	if got, want := effects.take(), []string{"slot-1"}; !slices.Equal(got, want) {
		t.Fatalf("Tick with past deadline = %v, want %v", got, want)
	}
}

func TestRespawnCancelStopsPendingRespawn(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, func() time.Time { return now })

	r.Add("slot-1", now.Add(time.Second))

	if !r.Cancel("slot-1") {
		t.Fatal("Cancel() = false, want true for tracked slot")
	}
	if r.Cancel("slot-1") {
		t.Fatal("Cancel() = true, want false for already-removed slot")
	}

	now = now.Add(time.Hour)
	r.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after cancel = %v, want none", got)
	}
}

func TestRespawnAddReplacesExistingDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, func() time.Time { return now })

	r.Add("slot-1", now.Add(time.Second))
	r.Add("slot-1", now.Add(10*time.Second))

	now = now.Add(time.Second)
	r.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before replaced deadline = %v, want none", got)
	}

	now = now.Add(9 * time.Second)
	r.Tick()
	if got, want := effects.take(), []string{"slot-1"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at replaced deadline = %v, want %v", got, want)
	}
}

func TestRespawnConcurrentAddAndTick(t *testing.T) {
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "slot"
			r.Add(key, time.Now())
			r.Tick()
			r.Cancel(key)
		}(i)
	}
	wg.Wait()
}
