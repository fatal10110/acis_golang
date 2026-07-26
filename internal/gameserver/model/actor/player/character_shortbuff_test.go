package player

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestCharacterUpdateShortBuffBroadcastsAndClearsAfterDuration(t *testing.T) {
	c := &Character{ID: 1}

	var mu sync.Mutex
	var updates []ShortBuffUpdate
	done := make(chan struct{}, 2)
	c.SetShortBuffBroadcaster(func(u ShortBuffUpdate) {
		mu.Lock()
		updates = append(updates, u)
		mu.Unlock()
		done <- struct{}{}
	})

	c.UpdateShortBuff(2031, 1, 1) // 1 second, short enough for a fast test
	<-done

	if got := c.ShortBuffTaskSkillID(); got != 2031 {
		t.Fatalf("ShortBuffTaskSkillID() = %d, want 2031", got)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the scheduled clear broadcast")
	}

	if got := c.ShortBuffTaskSkillID(); got != 0 {
		t.Fatalf("ShortBuffTaskSkillID() after clear = %d, want 0", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 {
		t.Fatalf("broadcast calls = %d, want 2 (start + clear)", len(updates))
	}
	if updates[0] != (ShortBuffUpdate{SkillID: 2031, Level: 1, DurationSeconds: 1}) {
		t.Fatalf("start update = %+v, want {2031 1 1}", updates[0])
	}
	if updates[1] != (ShortBuffUpdate{}) {
		t.Fatalf("clear update = %+v, want zero value", updates[1])
	}
}

// TestUpdateShortBuffRecoversPanickingClearCallback is the regression test
// for the panic class fixed in dispatch.go's scheduleAfter
// (network/dispatch_test.go's TestScheduleAfterRecoversPanickingCallback):
// the short-buff clear runs on its own goroutine via time.AfterFunc, outside
// any per-connection recover, so an unrecovered panic there (e.g. from a
// broadcaster hook) would kill the whole process instead of just this
// character.
func TestUpdateShortBuffRecoversPanickingClearCallback(t *testing.T) {
	buf := &syncCharBuffer{}
	c := &Character{ID: 1}
	c.SetLogger(zerolog.New(buf))
	c.SetShortBuffBroadcaster(func(u ShortBuffUpdate) {
		if u == (ShortBuffUpdate{}) {
			panic("boom")
		}
	})

	c.UpdateShortBuff(2031, 1, 1)

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(buf.String(), "boom") {
		if time.Now().After(deadline) {
			t.Fatalf("panic was not recovered and logged, got: %s", buf.String())
		}
		time.Sleep(time.Millisecond)
	}
}

// syncCharBuffer is a mutex-guarded bytes.Buffer safe for a test's polling
// goroutine to read while a scheduled callback's goroutine writes to it.
type syncCharBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncCharBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncCharBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestCharacterUpdateShortBuffCancelsPreviousTimer(t *testing.T) {
	c := &Character{ID: 1}

	var mu sync.Mutex
	var updates []ShortBuffUpdate
	c.SetShortBuffBroadcaster(func(u ShortBuffUpdate) {
		mu.Lock()
		defer mu.Unlock()
		updates = append(updates, u)
	})

	c.UpdateShortBuff(2031, 1, 100) // long duration; must not fire before the restart below
	c.UpdateShortBuff(2037, 1, 100)

	if got := c.ShortBuffTaskSkillID(); got != 2037 {
		t.Fatalf("ShortBuffTaskSkillID() = %d, want 2037 (the restarted buff)", got)
	}

	// Give any (incorrectly still-running) first timer a moment it
	// shouldn't need, then check no clear fired for skill 2031.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 {
		t.Fatalf("broadcast calls = %d, want 2 (two starts, no premature clear)", len(updates))
	}
}
